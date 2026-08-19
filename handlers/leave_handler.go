package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	utils.Success(w, http.StatusCreated, "Pengajuan cuti berhasil dikirim, menunggu persetujuan admin", map[string]interface{}{"id": id})
}

// GET /api/leaves  (Guru melihat riwayat pengajuannya sendiri)
func ListMyLeaves(w http.ResponseWriter, r *http.Request) {
	teacherID := middleware.GetUserID(r)

	rows, err := config.DB.Query(
		`SELECT id, teacher_id, start_date, end_date, leave_type, IFNULL(reason,''), status, created_at
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
		if err := rows.Scan(&l.ID, &l.TeacherID, &l.StartDate, &l.EndDate, &l.LeaveType, &l.Reason, &l.Status, &l.CreatedAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		leaves = append(leaves, l)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil riwayat cuti", leaves)
}

// GET /api/admin/leaves  (Admin melihat semua pengajuan cuti)
func ListAllLeaves(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(
		`SELECT l.id, l.teacher_id, u.name, l.start_date, l.end_date, l.leave_type, IFNULL(l.reason,''), l.status, l.created_at
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
		if err := rows.Scan(&l.ID, &l.TeacherID, &l.TeacherName, &l.StartDate, &l.EndDate, &l.LeaveType, &l.Reason, &l.Status, &l.CreatedAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		leaves = append(leaves, l)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil semua data cuti", leaves)
}

// PUT /api/admin/leaves/{id}/approve
func ApproveLeave(w http.ResponseWriter, r *http.Request) {
	updateLeaveStatus(w, r, "approved")
}

// PUT /api/admin/leaves/{id}/reject
func RejectLeave(w http.ResponseWriter, r *http.Request) {
	updateLeaveStatus(w, r, "rejected")
}

func updateLeaveStatus(w http.ResponseWriter, r *http.Request, status string) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}
	adminID := middleware.GetUserID(r)

	_, err = config.DB.Exec(
		`UPDATE leaves SET status = ?, approved_by = ?, approved_at = NOW() WHERE id = ?`,
		status, adminID, id,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memperbarui status cuti: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Status cuti berhasil diperbarui menjadi "+status, nil)
}
