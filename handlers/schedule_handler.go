package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"absensi-backend/config"
	"absensi-backend/models"
	"absensi-backend/utils"

	"github.com/gorilla/mux"
)

type ScheduleRequest struct {
	TeacherID   uint64 `json:"teacher_id"`
	RoomID      uint64 `json:"room_id"`
	DayOfWeek   int    `json:"day_of_week"`  // 1=Senin ... 7=Minggu
	PeriodMonth int    `json:"period_month"` // 1-12; kalau 0/kosong, default bulan berjalan
	PeriodYear  int    `json:"period_year"`  // kalau 0/kosong, default tahun berjalan
	StartTime   string `json:"start_time"`   // "HH:MM" atau "HH:MM:SS"
	EndTime     string `json:"end_time"`
	TargetJP    int    `json:"target_jp"`
	Subject     string `json:"subject"`
}

// GET /api/admin/schedules?month=8&year=2026
// Jadwal sekarang terikat bulan/periode -- tiap bulan bisa punya susunan
// jadwal yang berbeda, dan riwayat jadwal bulan-bulan lama tetap tersimpan
// (bisa dilihat lagi lewat filter bulan ini). Kalau month/year tidak
// dikirim, default menampilkan BULAN BERJALAN (paling relevan buat admin).
func ListSchedules(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	month := now.Month()
	year := now.Year()

	if m, err := strconv.Atoi(r.URL.Query().Get("month")); err == nil && m >= 1 && m <= 12 {
		month = time.Month(m)
	}
	if y, err := strconv.Atoi(r.URL.Query().Get("year")); err == nil && y > 0 {
		year = y
	}

	query := `
		SELECT s.id, s.teacher_id, u.name, s.room_id, rm.name, s.day_of_week,
		       s.period_month, s.period_year,
		       s.start_time, s.end_time, s.target_jp, IFNULL(s.subject, ''), s.is_active
		FROM schedules s
		JOIN users u ON u.id = s.teacher_id
		JOIN rooms rm ON rm.id = s.room_id
		WHERE s.period_month = ? AND s.period_year = ? AND s.is_active = 1
		ORDER BY s.day_of_week ASC, s.start_time ASC`

	rows, err := config.DB.Query(query, int(month), year)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil jadwal: "+err.Error())
		return
	}
	defer rows.Close()

	var schedules []models.Schedule
	for rows.Next() {
		var s models.Schedule
		if err := rows.Scan(&s.ID, &s.TeacherID, &s.TeacherName, &s.RoomID, &s.RoomName,
			&s.DayOfWeek, &s.PeriodMonth, &s.PeriodYear,
			&s.StartTime, &s.EndTime, &s.TargetJP, &s.Subject, &s.IsActive); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		schedules = append(schedules, s)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil jadwal", schedules)
}

// GET /api/admin/schedules/periods
// Daftar bulan-tahun yang punya jadwal tersimpan (dipakai untuk mengisi
// pilihan di filter "Bulan" di frontend -- termasuk riwayat bulan lama).
func ListSchedulePeriods(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(`
		SELECT DISTINCT period_year, period_month FROM schedules
		WHERE is_active = 1
		ORDER BY period_year DESC, period_month DESC`)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil daftar periode jadwal: "+err.Error())
		return
	}
	defer rows.Close()

	type period struct {
		Year  int `json:"year"`
		Month int `json:"month"`
	}
	var periods []period
	for rows.Next() {
		var p period
		if err := rows.Scan(&p.Year, &p.Month); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		periods = append(periods, p)
	}

	// Selalu ikutkan bulan berjalan di daftar, meski belum ada jadwalnya
	// sama sekali, supaya admin tetap bisa memilih "bulan ini" dari filter.
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

	utils.Success(w, http.StatusOK, "Berhasil mengambil daftar periode jadwal", periods)
}

// POST /api/admin/schedules
func CreateSchedule(w http.ResponseWriter, r *http.Request) {
	var req ScheduleRequest
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
		`INSERT INTO schedules (teacher_id, room_id, day_of_week, period_month, period_year, start_time, end_time, target_jp, subject)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.TeacherID, req.RoomID, req.DayOfWeek, req.PeriodMonth, req.PeriodYear,
		req.StartTime, req.EndTime, req.TargetJP, req.Subject,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal membuat jadwal: "+err.Error())
		return
	}

	id, _ := result.LastInsertId()
	utils.Success(w, http.StatusCreated, "Jadwal berhasil dibuat", map[string]interface{}{"id": id})
}

// PUT /api/admin/schedules/{id}
func UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var req ScheduleRequest
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
		`UPDATE schedules SET teacher_id=?, room_id=?, day_of_week=?, period_month=?, period_year=?,
		 start_time=?, end_time=?, target_jp=?, subject=?
		 WHERE id=?`,
		req.TeacherID, req.RoomID, req.DayOfWeek, req.PeriodMonth, req.PeriodYear,
		req.StartTime, req.EndTime, req.TargetJP, req.Subject, id,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memperbarui jadwal: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Jadwal berhasil diperbarui", nil)
}

// DELETE /api/admin/schedules/{id}
func DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	if _, err := config.DB.Exec(`UPDATE schedules SET is_active = 0 WHERE id = ?`, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menonaktifkan jadwal: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Jadwal berhasil dinonaktifkan", nil)
}

// POST /api/admin/schedules/{id}/duplicate
// Body: { "period_month": 9, "period_year": 2026 }
// Duplikasi 1 baris jadwal ke bulan lain -- shortcut supaya admin tidak
// perlu input ulang dari nol tiap bulan baru kalau susunannya sama/mirip.
func DuplicateSchedule(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var req struct {
		PeriodMonth int `json:"period_month"`
		PeriodYear  int `json:"period_year"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if req.PeriodMonth < 1 || req.PeriodMonth > 12 || req.PeriodYear <= 0 {
		utils.Error(w, http.StatusBadRequest, "period_month dan period_year wajib diisi dengan benar")
		return
	}

	result, err := config.DB.Exec(`
		INSERT INTO schedules (teacher_id, room_id, day_of_week, period_month, period_year, start_time, end_time, target_jp, subject)
		SELECT teacher_id, room_id, day_of_week, ?, ?, start_time, end_time, target_jp, subject
		FROM schedules WHERE id = ? AND is_active = 1`,
		req.PeriodMonth, req.PeriodYear, id,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menduplikasi jadwal: "+err.Error())
		return
	}

	newID, _ := result.LastInsertId()
	if newID == 0 {
		utils.Error(w, http.StatusNotFound, "Jadwal sumber tidak ditemukan")
		return
	}

	utils.Success(w, http.StatusCreated, "Jadwal berhasil diduplikasi ke periode baru", map[string]interface{}{"id": newID})
}
