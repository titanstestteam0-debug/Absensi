package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"absensi-backend/config"
	"absensi-backend/models"
	"absensi-backend/utils"

	"github.com/gorilla/mux"
)

// ---------------------------------------------------------------------
// Draft Jadwal -- pengganti fitur "Tahun Ajaran" lama. Sebuah draft
// dinamai bebas oleh admin (mis. "2026/2027 V1", "2026/2027 V2") dan
// berisi susunan jadwalnya SENDIRI (tabel draft_schedules), terpisah
// total dari jadwal yang sedang berlaku (tabel schedules) -- jadi admin
// bisa menyiapkan jadwal tahun ajaran depan tanpa mengganggu jadwal
// berjalan, sampai draft itu diaktifkan.
//
// Hanya SATU draft yang boleh aktif dalam satu waktu. Mengaktifkan draft
// menyalin seluruh isi draft_schedules milik draft itu ke tabel schedules
// (ditandai source_draft_id), sekaligus menonaktifkan draft lain yang
// sebelumnya aktif (dan membersihkan jadwal live hasil draft itu).
// Menonaktifkan sebuah draft menghapus lagi baris-baris di schedules yang
// berasal darinya, sehingga periode itu kembali kosong.
// ---------------------------------------------------------------------

// GET /api/admin/drafts
func ListDrafts(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(`
		SELECT d.id, d.name, d.is_active,
		       (SELECT COUNT(*) FROM draft_schedules ds WHERE ds.draft_id = d.id) AS schedule_count
		FROM schedule_drafts d
		ORDER BY d.is_active DESC, d.created_at DESC`)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil daftar draft: "+err.Error())
		return
	}
	defer rows.Close()

	drafts := []models.ScheduleDraft{}
	for rows.Next() {
		var d models.ScheduleDraft
		if err := rows.Scan(&d.ID, &d.Name, &d.IsActive, &d.ScheduleCount); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		drafts = append(drafts, d)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil daftar draft", drafts)
}

// POST /api/admin/drafts
// Body: { "name": "2026/2027 V1" }
func CreateDraft(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if req.Name == "" || len(req.Name) > 100 {
		utils.Error(w, http.StatusBadRequest, "Nama draft wajib diisi, maksimal 100 karakter")
		return
	}

	result, err := config.DB.Exec(`INSERT INTO schedule_drafts (name, is_active) VALUES (?, 0)`, req.Name)
	if err != nil {
		utils.Error(w, http.StatusConflict, "Gagal membuat draft (mungkin nama sudah dipakai): "+err.Error())
		return
	}

	id, _ := result.LastInsertId()
	utils.Success(w, http.StatusCreated, "Draft \""+req.Name+"\" berhasil dibuat", map[string]interface{}{"id": id})
}

// DELETE /api/admin/drafts/{id}
// Hanya boleh menghapus draft yang TIDAK sedang aktif (nonaktifkan dulu
// lewat endpoint deactivate kalau perlu), supaya tidak ada jadwal live
// yang tiba-tiba kehilangan sumbernya secara tidak sengaja.
func DeleteDraft(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var isActive bool
	err = config.DB.QueryRow(`SELECT is_active FROM schedule_drafts WHERE id = ?`, id).Scan(&isActive)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Draft tidak ditemukan")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memeriksa data: "+err.Error())
		return
	}
	if isActive {
		utils.Error(w, http.StatusBadRequest, "Draft ini sedang aktif. Nonaktifkan dulu sebelum menghapusnya.")
		return
	}

	// draft_schedules ikut terhapus otomatis (ON DELETE CASCADE)
	if _, err := config.DB.Exec(`DELETE FROM schedule_drafts WHERE id = ?`, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menghapus draft: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Draft berhasil dihapus", nil)
}

// PUT /api/admin/drafts/{id}/activate
// Menyalin seluruh isi draft ke jadwal LIVE (schedules), menandainya
// dengan source_draft_id supaya bisa dibersihkan lagi kalau draft ini
// dinonaktifkan nanti. Kalau ada draft lain yang sedang aktif, draft itu
// otomatis dinonaktifkan dulu (jadwal live hasil draft lama dibersihkan).
func ActivateDraft(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var name string
	if err := config.DB.QueryRow(`SELECT name FROM schedule_drafts WHERE id = ?`, id).Scan(&name); err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Draft tidak ditemukan")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memeriksa data: "+err.Error())
		return
	}

	tx, err := config.DB.Begin()
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memulai transaksi: "+err.Error())
		return
	}

	// Nonaktifkan draft lain yang sedang aktif (kalau ada) + bersihkan
	// jadwal live hasil draft itu, supaya cuma satu draft yang "berlaku".
	var oldActiveID uint64
	err = tx.QueryRow(`SELECT id FROM schedule_drafts WHERE is_active = 1 AND id != ?`, id).Scan(&oldActiveID)
	if err == nil {
		if _, err := tx.Exec(`DELETE FROM schedules WHERE source_draft_id = ?`, oldActiveID); err != nil {
			tx.Rollback()
			utils.Error(w, http.StatusInternalServerError, "Gagal membersihkan jadwal draft lama: "+err.Error())
			return
		}
		if _, err := tx.Exec(`UPDATE schedule_drafts SET is_active = 0 WHERE id = ?`, oldActiveID); err != nil {
			tx.Rollback()
			utils.Error(w, http.StatusInternalServerError, "Gagal menonaktifkan draft lama: "+err.Error())
			return
		}
	} else if err != sql.ErrNoRows {
		tx.Rollback()
		utils.Error(w, http.StatusInternalServerError, "Gagal memeriksa draft aktif: "+err.Error())
		return
	}

	// Kalau draft ini sendiri sebelumnya sudah pernah aktif lalu
	// dinonaktifkan lalu diaktifkan lagi, bersihkan dulu sisa lamanya
	// (mencegah baris dobel) sebelum menyalin ulang.
	if _, err := tx.Exec(`DELETE FROM schedules WHERE source_draft_id = ?`, id); err != nil {
		tx.Rollback()
		utils.Error(w, http.StatusInternalServerError, "Gagal membersihkan jadwal lama draft ini: "+err.Error())
		return
	}

	result, err := tx.Exec(`
		INSERT INTO schedules (source_draft_id, teacher_id, room_id, day_of_week, period_month, period_year, start_time, end_time, target_jp, subject)
		SELECT ?, teacher_id, room_id, day_of_week, period_month, period_year, start_time, end_time, target_jp, subject
		FROM draft_schedules WHERE draft_id = ?`, id, id)
	if err != nil {
		tx.Rollback()
		utils.Error(w, http.StatusInternalServerError, "Gagal menyalin jadwal draft ke jadwal live: "+err.Error())
		return
	}
	copied, _ := result.RowsAffected()

	if _, err := tx.Exec(`UPDATE schedule_drafts SET is_active = 1 WHERE id = ?`, id); err != nil {
		tx.Rollback()
		utils.Error(w, http.StatusInternalServerError, "Gagal mengaktifkan draft: "+err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menyimpan perubahan: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Draft \""+name+"\" berhasil diaktifkan", map[string]interface{}{
		"schedules_applied": copied,
	})
}

// PUT /api/admin/drafts/{id}/deactivate
// Menghapus lagi jadwal LIVE yang berasal dari draft ini (source_draft_id),
// mengembalikan periode itu jadi kosong seperti sebelum draft diaktifkan.
func DeactivateDraft(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var name string
	var isActive bool
	err = config.DB.QueryRow(`SELECT name, is_active FROM schedule_drafts WHERE id = ?`, id).Scan(&name, &isActive)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Draft tidak ditemukan")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memeriksa data: "+err.Error())
		return
	}
	if !isActive {
		utils.Error(w, http.StatusBadRequest, "Draft ini sedang tidak aktif")
		return
	}

	tx, err := config.DB.Begin()
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memulai transaksi: "+err.Error())
		return
	}
	result, err := tx.Exec(`DELETE FROM schedules WHERE source_draft_id = ?`, id)
	if err != nil {
		tx.Rollback()
		utils.Error(w, http.StatusInternalServerError, "Gagal membersihkan jadwal live: "+err.Error())
		return
	}
	removed, _ := result.RowsAffected()
	if _, err := tx.Exec(`UPDATE schedule_drafts SET is_active = 0 WHERE id = ?`, id); err != nil {
		tx.Rollback()
		utils.Error(w, http.StatusInternalServerError, "Gagal menonaktifkan draft: "+err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menyimpan perubahan: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Draft \""+name+"\" berhasil dinonaktifkan", map[string]interface{}{
		"schedules_removed": removed,
	})
}

// ---------------------------------------------------------------------
// Jadwal DI DALAM sebuah draft -- mirip persis endpoint /admin/schedules,
// tapi beroperasi di draft_schedules dan selalu di-scope ke {draftId}.
// ---------------------------------------------------------------------

type DraftScheduleRequest struct {
	TeacherID   uint64 `json:"teacher_id"`
	RoomID      uint64 `json:"room_id"`
	DayOfWeek   int    `json:"day_of_week"`
	PeriodMonth int    `json:"period_month"`
	PeriodYear  int    `json:"period_year"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	TargetJP    int    `json:"target_jp"`
	Subject     string `json:"subject"`
}

// GET /api/admin/drafts/{draftId}/schedules?month=9&year=2026
func ListDraftSchedules(w http.ResponseWriter, r *http.Request) {
	draftID, err := strconv.ParseUint(mux.Vars(r)["draftId"], 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID draft tidak valid")
		return
	}

	now := time.Now()
	month := int(now.Month())
	year := now.Year()
	if m, err := strconv.Atoi(r.URL.Query().Get("month")); err == nil && m >= 1 && m <= 12 {
		month = m
	}
	if y, err := strconv.Atoi(r.URL.Query().Get("year")); err == nil && y > 0 {
		year = y
	}

	rows, err := config.DB.Query(`
		SELECT ds.id, ds.draft_id, ds.teacher_id, u.name, ds.room_id, rm.name, ds.day_of_week,
		       ds.period_month, ds.period_year, ds.start_time, ds.end_time, ds.target_jp, IFNULL(ds.subject, '')
		FROM draft_schedules ds
		JOIN users u ON u.id = ds.teacher_id
		JOIN rooms rm ON rm.id = ds.room_id
		WHERE ds.draft_id = ? AND ds.period_month = ? AND ds.period_year = ?
		ORDER BY ds.day_of_week ASC, ds.start_time ASC`, draftID, month, year)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil jadwal draft: "+err.Error())
		return
	}
	defer rows.Close()

	items := []models.DraftSchedule{}
	for rows.Next() {
		var s models.DraftSchedule
		if err := rows.Scan(&s.ID, &s.DraftID, &s.TeacherID, &s.TeacherName, &s.RoomID, &s.RoomName,
			&s.DayOfWeek, &s.PeriodMonth, &s.PeriodYear, &s.StartTime, &s.EndTime, &s.TargetJP, &s.Subject); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		items = append(items, s)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil jadwal draft", items)
}

// GET /api/admin/drafts/{draftId}/schedules/periods
func ListDraftSchedulePeriods(w http.ResponseWriter, r *http.Request) {
	draftID, err := strconv.ParseUint(mux.Vars(r)["draftId"], 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID draft tidak valid")
		return
	}

	rows, err := config.DB.Query(`
		SELECT DISTINCT period_year, period_month FROM draft_schedules
		WHERE draft_id = ?
		ORDER BY period_year DESC, period_month DESC`, draftID)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil daftar periode: "+err.Error())
		return
	}
	defer rows.Close()

	type period struct {
		Year  int `json:"year"`
		Month int `json:"month"`
	}
	periods := []period{}
	for rows.Next() {
		var p period
		if err := rows.Scan(&p.Year, &p.Month); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		periods = append(periods, p)
	}

	now := time.Now()
	hasCurrent := false
	for _, p := range periods {
		if p.Year == now.Year() && p.Month == int(now.Month()) {
			hasCurrent = true
			break
		}
	}
	if !hasCurrent {
		periods = append([]period{{Year: now.Year(), Month: int(now.Month())}}, periods...)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil daftar periode draft", periods)
}

// POST /api/admin/drafts/{draftId}/schedules
func CreateDraftSchedule(w http.ResponseWriter, r *http.Request) {
	draftID, err := strconv.ParseUint(mux.Vars(r)["draftId"], 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID draft tidak valid")
		return
	}

	var req DraftScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if req.TeacherID == 0 || req.RoomID == 0 || req.DayOfWeek < 1 || req.DayOfWeek > 7 ||
		req.StartTime == "" || req.EndTime == "" || req.TargetJP <= 0 {
		utils.Error(w, http.StatusBadRequest, "Semua field wajib diisi dengan benar")
		return
	}
	now := time.Now()
	if req.PeriodMonth < 1 || req.PeriodMonth > 12 {
		req.PeriodMonth = int(now.Month())
	}
	if req.PeriodYear <= 0 {
		req.PeriodYear = now.Year()
	}

	result, err := config.DB.Exec(
		`INSERT INTO draft_schedules (draft_id, teacher_id, room_id, day_of_week, period_month, period_year, start_time, end_time, target_jp, subject)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		draftID, req.TeacherID, req.RoomID, req.DayOfWeek, req.PeriodMonth, req.PeriodYear,
		req.StartTime, req.EndTime, req.TargetJP, req.Subject,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal membuat jadwal draft: "+err.Error())
		return
	}

	id, _ := result.LastInsertId()
	utils.Success(w, http.StatusCreated, "Jadwal draft berhasil dibuat", map[string]interface{}{"id": id})
}

// PUT /api/admin/drafts/{draftId}/schedules/{id}
func UpdateDraftSchedule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	draftID, err := strconv.ParseUint(vars["draftId"], 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID draft tidak valid")
		return
	}
	id, err := strconv.ParseUint(vars["id"], 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var req DraftScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	now := time.Now()
	if req.PeriodMonth < 1 || req.PeriodMonth > 12 {
		req.PeriodMonth = int(now.Month())
	}
	if req.PeriodYear <= 0 {
		req.PeriodYear = now.Year()
	}

	_, err = config.DB.Exec(
		`UPDATE draft_schedules SET teacher_id=?, room_id=?, day_of_week=?, period_month=?, period_year=?,
		 start_time=?, end_time=?, target_jp=?, subject=?
		 WHERE id=? AND draft_id=?`,
		req.TeacherID, req.RoomID, req.DayOfWeek, req.PeriodMonth, req.PeriodYear,
		req.StartTime, req.EndTime, req.TargetJP, req.Subject, id, draftID,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memperbarui jadwal draft: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Jadwal draft berhasil diperbarui", nil)
}

// DELETE /api/admin/drafts/{draftId}/schedules/{id}
// Hard delete (bukan soft-delete seperti jadwal live) -- draft belum
// "berlaku", jadi tidak perlu jejak riwayat penghapusan.
func DeleteDraftSchedule(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	draftID, err := strconv.ParseUint(vars["draftId"], 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID draft tidak valid")
		return
	}
	id, err := strconv.ParseUint(vars["id"], 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	if _, err := config.DB.Exec(`DELETE FROM draft_schedules WHERE id = ? AND draft_id = ?`, id, draftID); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menghapus jadwal draft: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Jadwal draft berhasil dihapus", nil)
}
