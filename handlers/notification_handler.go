package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"absensi-backend/config"
	"absensi-backend/middleware"
	"absensi-backend/models"
	"absensi-backend/utils"

	"github.com/gorilla/mux"
)

// Tipe notifikasi (kolom notifications.type). Frontend memakai nilai ini
// untuk menentukan label & warna badge, jadi kalau menambah tipe baru cukup
// tambahkan konstanta di sini lalu petakan di frontend.
const (
	NotifLeaveSubmitted    = "leave_submitted"     // guru mengajukan cuti -> ke admin
	NotifLeaveApproved     = "leave_approved"      // cuti disetujui -> ke guru
	NotifLeaveRejected     = "leave_rejected"      // cuti ditolak -> ke guru
	NotifLeaveAdminCreated = "leave_admin_created" // admin mengajukan cuti atas nama guru -> ke guru
)

// ---------------------------------------------------------------------
// Helper pembuat notifikasi -- dipanggil dari handler lain (mis. cuti).
// Kegagalan membuat notifikasi TIDAK boleh menggagalkan aksi utamanya
// (cuti tetap tersimpan), jadi error cukup dicatat di log.
// ---------------------------------------------------------------------

// createNotification menyimpan satu notifikasi untuk satu penerima.
// refID = 0 berarti tidak ada entitas terkait.
func createNotification(userID uint64, ntype, title, message string, refID uint64) {
	var ref interface{}
	if refID != 0 {
		ref = refID
	}
	// created_at diisi dari Go (bukan DEFAULT CURRENT_TIMESTAMP) supaya
	// konsisten dengan timezone aplikasi (Asia/Jakarta) di semua hosting.
	_, err := config.DB.Exec(
		`INSERT INTO notifications (user_id, type, title, message, ref_id, is_read, created_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?)`,
		userID, ntype, title, message, ref, time.Now(),
	)
	if err != nil {
		log.Printf("gagal membuat notifikasi (user %d, tipe %s): %v", userID, ntype, err)
	}
}

// notifyAllAdmins mengirim notifikasi yang sama ke semua admin aktif.
func notifyAllAdmins(ntype, title, message string, refID uint64) {
	rows, err := config.DB.Query(`SELECT id FROM users WHERE role = 'admin' AND is_active = 1`)
	if err != nil {
		log.Printf("gagal mengambil daftar admin untuk notifikasi: %v", err)
		return
	}
	var adminIDs []uint64
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err == nil {
			adminIDs = append(adminIDs, id)
		}
	}
	rows.Close()

	for _, id := range adminIDs {
		createNotification(id, ntype, title, message, refID)
	}
}

var bulanIndonesia = []string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember"}

// formatTanggalID mengubah "2026-09-21" menjadi "21 September 2026".
// Kalau formatnya tak dikenali, nilai aslinya dikembalikan apa adanya.
func formatTanggalID(s string) string {
	if len(s) >= 10 {
		s = s[:10]
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return s
	}
	return strconv.Itoa(t.Day()) + " " + bulanIndonesia[int(t.Month())] + " " + strconv.Itoa(t.Year())
}

// formatRentangTanggalID: satu tanggal kalau start == end, selain itu "A s/d B".
func formatRentangTanggalID(start, end string) string {
	a, b := formatTanggalID(start), formatTanggalID(end)
	if a == b {
		return a
	}
	return a + " s/d " + b
}

// leaveTypeLabelFor mengambil label jenis cuti dari master data
// (mis. "dinas_luar" -> "Izin Dinas Luar"); fallback ke kode-nya.
func leaveTypeLabelFor(code string) string {
	var label string
	if err := config.DB.QueryRow(`SELECT label FROM leave_types WHERE code = ?`, code).Scan(&label); err != nil || label == "" {
		return code
	}
	return label
}

// ---------------------------------------------------------------------
// Endpoint untuk semua user yang login (admin, guru, guru_pengganti):
// melihat & menandai notifikasi MILIKNYA sendiri.
// ---------------------------------------------------------------------

// GET /api/notifications?limit=30
// Mengembalikan notifikasi milik user yang login (terbaru dulu) beserta
// jumlah yang belum dibaca (untuk badge lonceng di header).
func ListMyNotifications(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	limit := 30
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}

	var unread int
	if err := config.DB.QueryRow(
		`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND is_read = 0`, userID,
	).Scan(&unread); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menghitung notifikasi: "+err.Error())
		return
	}

	rows, err := config.DB.Query(
		`SELECT id, user_id, type, title, message, ref_id, is_read, read_at, created_at
		 FROM notifications WHERE user_id = ?
		 ORDER BY created_at DESC, id DESC LIMIT ?`, userID, limit,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil notifikasi: "+err.Error())
		return
	}
	defer rows.Close()

	items := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Message, &n.RefID, &n.IsRead, &n.ReadAt, &n.CreatedAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca notifikasi: "+err.Error())
			return
		}
		items = append(items, n)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil notifikasi", map[string]interface{}{
		"unread_count": unread,
		"items":        items,
	})
}

// PUT /api/notifications/{id}/read
// Menandai satu notifikasi sebagai sudah dibaca. Hanya bisa untuk notifikasi
// milik user yang login sendiri -- notifikasi orang lain dianggap tidak ada.
func MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}
	userID := middleware.GetUserID(r)

	var isRead bool
	err = config.DB.QueryRow(
		`SELECT is_read FROM notifications WHERE id = ? AND user_id = ?`, id, userID,
	).Scan(&isRead)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Notifikasi tidak ditemukan")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memeriksa notifikasi: "+err.Error())
		return
	}

	// Sudah dibaca sebelumnya -> biarkan read_at yang lama (waktu baca pertama).
	if !isRead {
		if _, err := config.DB.Exec(
			`UPDATE notifications SET is_read = 1, read_at = ? WHERE id = ? AND user_id = ?`,
			time.Now(), id, userID,
		); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal menandai notifikasi: "+err.Error())
			return
		}
	}

	utils.Success(w, http.StatusOK, "Notifikasi ditandai sudah dibaca", nil)
}

// PUT /api/notifications/read-all
// Menandai SEMUA notifikasi milik user yang login sebagai sudah dibaca.
func MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	if _, err := config.DB.Exec(
		`UPDATE notifications SET is_read = 1, read_at = ? WHERE user_id = ? AND is_read = 0`,
		time.Now(), userID,
	); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menandai semua notifikasi: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Semua notifikasi ditandai sudah dibaca", nil)
}

// ---------------------------------------------------------------------
// Endpoint khusus Admin: melihat SEMUA notifikasi yang dikirim ke semua
// pengguna beserta status baca-nya (tabel di halaman Data Ruangan).
// ---------------------------------------------------------------------

// GET /api/admin/notifications?status=all|unread|read&limit=100
// summary dihitung dari SELURUH notifikasi (bukan hanya yang muncul di
// items), supaya angka ringkasan tetap benar walau daftar dibatasi limit.
func AdminListNotifications(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}

	where := ""
	switch strings.ToLower(r.URL.Query().Get("status")) {
	case "unread":
		where = "WHERE n.is_read = 0"
	case "read":
		where = "WHERE n.is_read = 1"
	}

	var total, unread int
	if err := config.DB.QueryRow(
		`SELECT COUNT(*), IFNULL(SUM(CASE WHEN is_read = 0 THEN 1 ELSE 0 END), 0) FROM notifications`,
	).Scan(&total, &unread); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menghitung notifikasi: "+err.Error())
		return
	}

	rows, err := config.DB.Query(
		`SELECT n.id, n.user_id, u.name, u.role, n.type, n.title, n.message, n.ref_id, n.is_read, n.read_at, n.created_at
		 FROM notifications n JOIN users u ON u.id = n.user_id `+where+`
		 ORDER BY n.created_at DESC, n.id DESC LIMIT ?`, limit,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil notifikasi: "+err.Error())
		return
	}
	defer rows.Close()

	items := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.UserName, &n.UserRole, &n.Type, &n.Title, &n.Message, &n.RefID, &n.IsRead, &n.ReadAt, &n.CreatedAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca notifikasi: "+err.Error())
			return
		}
		items = append(items, n)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil daftar notifikasi", map[string]interface{}{
		"summary": map[string]int{
			"total":  total,
			"unread": unread,
			"read":   total - unread,
		},
		"items": items,
	})
}
