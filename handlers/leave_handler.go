package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"absensi-backend/config"
	"absensi-backend/middleware"
	"absensi-backend/models"
	"absensi-backend/utils"

	"github.com/gorilla/mux"
)

type LeaveRequest struct {
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	LeaveType     string `json:"leave_type"`
	Reason        string `json:"reason"`
	AttachmentURL string `json:"attachment_url"`
}

// AdminLeaveRequest sama seperti LeaveRequest tapi admin juga menentukan
// guru mana yang diajukan cutinya.
type AdminLeaveRequest struct {
	TeacherID     uint64 `json:"teacher_id"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	LeaveType     string `json:"leave_type"`
	Reason        string `json:"reason"`
	AttachmentURL string `json:"attachment_url"`
}

type RejectLeaveRequest struct {
	Reason string `json:"reason"`
}

// POST /api/leaves  (Guru mengajukan cuti/izin)
func CreateLeave(w http.ResponseWriter, r *http.Request) {
	teacherID := middleware.GetUserID(r)

	var req LeaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if req.StartDate == "" || req.EndDate == "" || req.LeaveType == "" {
		utils.Error(w, http.StatusBadRequest, "start_date, end_date, dan leave_type wajib diisi")
		return
	}

	result, err := config.DB.Exec(
		`INSERT INTO leaves (teacher_id, start_date, end_date, leave_type, reason, attachment_url, status)
		 VALUES (?, ?, ?, ?, ?, ?, 'pending')`,
		teacherID, req.StartDate, req.EndDate, req.LeaveType, req.Reason, req.AttachmentURL,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengajukan cuti: "+err.Error())
		return
	}

	id, _ := result.LastInsertId()

	// Beri tahu semua admin bahwa ada pengajuan baru yang menunggu persetujuan.
	var teacherName string
	config.DB.QueryRow(`SELECT name FROM users WHERE id = ?`, teacherID).Scan(&teacherName)
	if teacherName == "" {
		teacherName = "Seorang guru"
	}
	notifyAllAdmins(
		NotifLeaveSubmitted,
		"Pengajuan cuti/izin baru",
		teacherName+" mengajukan "+leaveTypeLabelFor(req.LeaveType)+" pada "+formatRentangTanggalID(req.StartDate, req.EndDate)+". Menunggu persetujuan Anda.",
		uint64(id),
	)

	utils.Success(w, http.StatusCreated, "Pengajuan cuti berhasil dikirim, menunggu persetujuan admin", map[string]interface{}{"id": id})
}

// GET /api/leaves  (Guru melihat riwayat pengajuannya sendiri)
func ListMyLeaves(w http.ResponseWriter, r *http.Request) {
	teacherID := middleware.GetUserID(r)

	rows, err := config.DB.Query(
		`SELECT id, teacher_id, start_date, end_date, leave_type, IFNULL(reason,''), status, IFNULL(rejection_reason,''), created_at
		 FROM leaves WHERE teacher_id = ? ORDER BY created_at DESC`, teacherID,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data cuti: "+err.Error())
		return
	}
	defer rows.Close()

	var leaves []models.Leave
	for rows.Next() {
		var l models.Leave
		if err := rows.Scan(&l.ID, &l.TeacherID, &l.StartDate, &l.EndDate, &l.LeaveType, &l.Reason, &l.Status, &l.RejectionReason, &l.CreatedAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		leaves = append(leaves, l)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil riwayat cuti", leaves)
}

// POST /api/admin/leaves
// Admin membantu mengajukan cuti/izin atas nama seorang guru, LANGSUNG
// berstatus 'approved' -- tanpa perlu melalui alur persetujuan pending
// seperti pengajuan mandiri oleh guru (mis. untuk kondisi darurat/mendadak
// yang sudah dikomunikasikan langsung ke admin/kepala sekolah).
func AdminCreateLeave(w http.ResponseWriter, r *http.Request) {
	adminID := middleware.GetUserID(r)

	var req AdminLeaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if req.TeacherID == 0 || req.StartDate == "" || req.EndDate == "" || req.LeaveType == "" {
		utils.Error(w, http.StatusBadRequest, "teacher_id, start_date, end_date, dan leave_type wajib diisi")
		return
	}

	// Pastikan guru yang dituju benar-benar ada (dan bukan sesama admin).
	var teacherName, teacherRole string
	err := config.DB.QueryRow(`SELECT name, role FROM users WHERE id = ?`, req.TeacherID).
		Scan(&teacherName, &teacherRole)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Guru tidak ditemukan")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memeriksa data guru: "+err.Error())
		return
	}
	if teacherRole != string(models.RoleGuru) && teacherRole != string(models.RoleGuruPengganti) {
		utils.Error(w, http.StatusBadRequest, "Cuti hanya bisa diajukan untuk akun bertipe guru/guru pengganti")
		return
	}

	result, err := config.DB.Exec(
		`INSERT INTO leaves (teacher_id, start_date, end_date, leave_type, reason, attachment_url, status, approved_by, approved_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'approved', ?, NOW())`,
		req.TeacherID, req.StartDate, req.EndDate, req.LeaveType, req.Reason, req.AttachmentURL, adminID,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengajukan cuti untuk guru: "+err.Error())
		return
	}

	id, _ := result.LastInsertId()

	// Beri tahu guru bahwa admin sudah mencatat (dan menyetujui) cutinya.
	createNotification(
		req.TeacherID,
		NotifLeaveAdminCreated,
		"Cuti/izin dicatat oleh admin",
		"Admin telah mengajukan "+leaveTypeLabelFor(req.LeaveType)+" untuk Anda pada "+formatRentangTanggalID(req.StartDate, req.EndDate)+" dan langsung disetujui.",
		uint64(id),
	)

	utils.Success(w, http.StatusCreated, "Cuti untuk "+teacherName+" berhasil diajukan & otomatis disetujui", map[string]interface{}{
		"id":         id,
		"teacher_id": req.TeacherID,
		"status":     "approved",
	})
}

// GET /api/admin/leaves  (Admin melihat semua pengajuan cuti)
func ListAllLeaves(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(
		`SELECT l.id, l.teacher_id, u.name, l.start_date, l.end_date, l.leave_type, IFNULL(l.reason,''), l.status, IFNULL(l.rejection_reason,''), l.created_at
		 FROM leaves l JOIN users u ON u.id = l.teacher_id
		 ORDER BY l.created_at DESC`,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data cuti: "+err.Error())
		return
	}
	defer rows.Close()

	type leaveWithName struct {
		models.Leave
		TeacherName string `json:"teacher_name"`
	}

	var leaves []leaveWithName
	for rows.Next() {
		var l leaveWithName
		if err := rows.Scan(&l.ID, &l.TeacherID, &l.TeacherName, &l.StartDate, &l.EndDate, &l.LeaveType, &l.Reason, &l.Status, &l.RejectionReason, &l.CreatedAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		leaves = append(leaves, l)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil semua data cuti", leaves)
}

// PUT /api/admin/leaves/{id}/approve
func ApproveLeave(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}
	adminID := middleware.GetUserID(r)

	// Ambil data cuti SEBELUM diubah, untuk isi notifikasi & supaya tidak
	// mengirim notifikasi ganda kalau statusnya memang sudah 'approved'.
	info, infoErr := getLeaveNotifInfo(id)

	// rejection_reason di-NULL-kan lagi kalau sebelumnya pernah ditolak lalu
	// disetujui ulang, supaya tidak ada alasan penolakan basi yang nyangkut.
	_, err = config.DB.Exec(
		`UPDATE leaves SET status = 'approved', rejection_reason = NULL, approved_by = ?, approved_at = NOW() WHERE id = ?`,
		adminID, id,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menyetujui cuti: "+err.Error())
		return
	}

	if infoErr == nil && info.Status != string(models.LeaveApproved) {
		createNotification(
			info.TeacherID,
			NotifLeaveApproved,
			"Pengajuan cuti/izin disetujui",
			"Pengajuan "+leaveTypeLabelFor(info.LeaveType)+" Anda pada "+formatRentangTanggalID(info.StartDate, info.EndDate)+" telah disetujui.",
			id,
		)
	}

	utils.Success(w, http.StatusOK, "Cuti berhasil disetujui", nil)
}

// PUT /api/admin/leaves/{id}/reject
// Admin wajib mengisi alasan penolakan, supaya guru tahu kenapa pengajuannya ditolak.
func RejectLeave(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var req RejectLeaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		utils.Error(w, http.StatusBadRequest, "Alasan penolakan wajib diisi")
		return
	}

	adminID := middleware.GetUserID(r)

	info, infoErr := getLeaveNotifInfo(id)

	_, err = config.DB.Exec(
		`UPDATE leaves SET status = 'rejected', rejection_reason = ?, approved_by = ?, approved_at = NOW() WHERE id = ?`,
		req.Reason, adminID, id,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menolak cuti: "+err.Error())
		return
	}

	if infoErr == nil {
		createNotification(
			info.TeacherID,
			NotifLeaveRejected,
			"Pengajuan cuti/izin ditolak",
			"Pengajuan "+leaveTypeLabelFor(info.LeaveType)+" Anda pada "+formatRentangTanggalID(info.StartDate, info.EndDate)+" ditolak. Alasan: "+req.Reason,
			id,
		)
	}

	utils.Success(w, http.StatusOK, "Cuti berhasil ditolak", nil)
}

// leaveNotifInfo: ringkasan data cuti yang dibutuhkan untuk menyusun isi notifikasi.
type leaveNotifInfo struct {
	TeacherID uint64
	LeaveType string
	StartDate string
	EndDate   string
	Status    string
}

// getLeaveNotifInfo membaca data cuti (guru, jenis, tanggal, status saat ini).
func getLeaveNotifInfo(leaveID uint64) (leaveNotifInfo, error) {
	var i leaveNotifInfo
	err := config.DB.QueryRow(
		`SELECT teacher_id, leave_type, start_date, end_date, status FROM leaves WHERE id = ?`, leaveID,
	).Scan(&i.TeacherID, &i.LeaveType, &i.StartDate, &i.EndDate, &i.Status)
	return i, err
}
