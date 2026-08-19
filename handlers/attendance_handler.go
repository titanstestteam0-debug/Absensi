package handlers

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	"absensi-backend/config"
	"absensi-backend/middleware"
	"absensi-backend/utils"
)

// getSettingInt membaca nilai integer dari tabel settings (mis. jp_duration_minutes).
func getSettingInt(key string, fallback int) int {
	var value string
	err := config.DB.QueryRow(`SELECT config_value FROM settings WHERE config_key = ?`, key).Scan(&value)
	if err != nil {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

// weekdayToDayOfWeek mengonversi time.Weekday (0=Minggu..6=Sabtu) ke skema
// day_of_week milik aplikasi (1=Senin ... 7=Minggu).
func weekdayToDayOfWeek(wd time.Weekday) int {
	if wd == time.Sunday {
		return 7
	}
	return int(wd) // Monday=1 ... Saturday=6
}

// hasApprovedLeaveToday memeriksa apakah guru utama sedang dalam rentang cuti
// yang berstatus 'approved' pada hari ini, syarat wajib supaya Guru Pengganti
// bisa presensi menggantikannya.
func hasApprovedLeaveToday(teacherID uint64, now time.Time) (bool, error) {
	var count int
	err := config.DB.QueryRow(
		`SELECT COUNT(*) FROM leaves
		 WHERE teacher_id = ? AND status = 'approved' AND ? BETWEEN start_date AND end_date`,
		teacherID, now.Format("2006-01-02"),
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

type scheduleRow struct {
	ID        uint64
	TeacherID uint64
	StartTime string // "HH:MM:SS"
	EndTime   string
	TargetJP  int
}

// parseClockToday menggabungkan tanggal hari ini dengan jam "HH:MM:SS" dari kolom TIME MySQL.
func parseClockToday(now time.Time, hhmmss string) (time.Time, error) {
	t, err := time.ParseInLocation("15:04:05", hhmmss, now.Location())
	if err != nil {
		// fallback jika format "HH:MM"
		t, err = time.ParseInLocation("15:04", hhmmss, now.Location())
		if err != nil {
			return time.Time{}, err
		}
	}
	return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location()), nil
}

// =====================================================================
// POST /api/attendance/scan-in
// Body: { "qr_string": "...", "substitute_for_id": 123 }  // substitute_for_id opsional
// =====================================================================
func ScanIn(w http.ResponseWriter, r *http.Request) {
	authUserID := middleware.GetUserID(r)

	var req struct {
		QRString        string  `json:"qr_string"`
		SubstituteForID *uint64 `json:"substitute_for_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if req.QRString == "" {
		utils.Error(w, http.StatusBadRequest, "qr_string wajib diisi")
		return
	}

	// PENTING: waktu SELALU diambil dari server, tidak pernah dari device (anti fake clock).
	now := time.Now()

	var roomID uint64
	err := config.DB.QueryRow(`SELECT id FROM rooms WHERE qr_string = ? AND is_active = 1`, req.QRString).Scan(&roomID)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "QR Code ruangan tidak dikenali")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal validasi ruangan: "+err.Error())
		return
	}

	// Guru mana yang jadwalnya dicari: guru utama, atau guru yang digantikan (mode inval).
	scheduleTeacherID := authUserID
	var substituteID *uint64
	if req.SubstituteForID != nil && *req.SubstituteForID != 0 {
		scheduleTeacherID = *req.SubstituteForID
		substituteID = &authUserID

		// Mode Guru Pengganti hanya boleh dipakai jika guru utama memang sedang
		// dalam status cuti/izin yang SUDAH disetujui pada tanggal hari ini.
		approved, err := hasApprovedLeaveToday(scheduleTeacherID, now)
		if err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal memeriksa status cuti guru utama: "+err.Error())
			return
		}
		if !approved {
			utils.Error(w, http.StatusForbidden, "Guru utama tidak dalam status cuti yang disetujui hari ini, tidak bisa presensi sebagai pengganti")
			return
		}
	}

	dow := weekdayToDayOfWeek(now.Weekday())
	tolerance := getSettingInt("early_scan_tolerance_minutes", 60)

	rows, err := config.DB.Query(
		`SELECT id, teacher_id, start_time, end_time, target_jp
		 FROM schedules
		 WHERE teacher_id = ? AND room_id = ? AND day_of_week = ? AND is_active = 1
		 ORDER BY start_time ASC`,
		scheduleTeacherID, roomID, dow,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mencari jadwal: "+err.Error())
		return
	}
	defer rows.Close()

	var candidates []scheduleRow
	for rows.Next() {
		var s scheduleRow
		if err := rows.Scan(&s.ID, &s.TeacherID, &s.StartTime, &s.EndTime, &s.TargetJP); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca jadwal: "+err.Error())
			return
		}
		candidates = append(candidates, s)
	}

	if len(candidates) == 0 {
		utils.Error(w, http.StatusNotFound, "Tidak ada jadwal untuk guru ini di ruangan tersebut hari ini")
		return
	}

	// Cari jadwal terdekat: yang sedang berlangsung, atau yang terdekat ke depan
	// dan sudah masuk masa toleransi scan awal.
	var chosen *scheduleRow
	for i := range candidates {
		s := candidates[i]
		startAt, err := parseClockToday(now, s.StartTime)
		if err != nil {
			continue
		}
		endAt, err := parseClockToday(now, s.EndTime)
		if err != nil {
			continue
		}
		toleranceStart := startAt.Add(-time.Duration(tolerance) * time.Minute)

		if now.Before(endAt) && !now.Before(toleranceStart) {
			chosen = &s
			break
		}
	}

	if chosen == nil {
		utils.Error(w, http.StatusBadRequest, "Jadwal Anda belum dimulai")
		return
	}

	// Cegah scan-in dobel untuk jadwal & tanggal yang sama.
	dateStr := now.Format("2006-01-02")
	var existingID uint64
	err = config.DB.QueryRow(
		`SELECT id FROM attendances WHERE schedule_id = ? AND date = ?`, chosen.ID, dateStr,
	).Scan(&existingID)
	if err == nil {
		utils.Error(w, http.StatusConflict, "Anda sudah melakukan scan masuk untuk jadwal ini hari ini")
		return
	} else if err != sql.ErrNoRows {
		utils.Error(w, http.StatusInternalServerError, "Gagal memeriksa presensi: "+err.Error())
		return
	}

	result, err := config.DB.Exec(
		`INSERT INTO attendances (schedule_id, teacher_id, substitute_teacher_id, date, clock_in, status, room_id_scanned)
		 VALUES (?, ?, ?, ?, ?, 'in_progress', ?)`,
		chosen.ID, chosen.TeacherID, substituteID, dateStr, now, roomID,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menyimpan presensi: "+err.Error())
		return
	}

	id, _ := result.LastInsertId()
	utils.Success(w, http.StatusCreated, "Scan masuk berhasil dicatat", map[string]interface{}{
		"attendance_id": id,
		"schedule_id":   chosen.ID,
		"clock_in":      now.Format(time.RFC3339),
		"bound_to":      chosen.StartTime, // jadwal yang diikat, bukan jam scan aktual
	})
}

// =====================================================================
// POST /api/attendance/scan-out
// Body: { "qr_string": "..." }
// =====================================================================
func ScanOut(w http.ResponseWriter, r *http.Request) {
	authUserID := middleware.GetUserID(r)

	var req struct {
		QRString string `json:"qr_string"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}

	now := time.Now()
	dateStr := now.Format("2006-01-02")

	var roomID uint64
	err := config.DB.QueryRow(`SELECT id FROM rooms WHERE qr_string = ? AND is_active = 1`, req.QRString).Scan(&roomID)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "QR Code ruangan tidak dikenali")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal validasi ruangan: "+err.Error())
		return
	}

	// Cari attendance yang sedang berjalan (in_progress) untuk guru ini hari ini di ruangan ini.
	var (
		attID      uint64
		scheduleID uint64
		clockIn    time.Time
		startTime  string
		endTime    string
		targetJP   int
	)
	query := `
		SELECT a.id, a.schedule_id, a.clock_in, s.start_time, s.end_time, s.target_jp
		FROM attendances a
		JOIN schedules s ON s.id = a.schedule_id
		WHERE a.date = ? AND a.room_id_scanned = ? AND a.status = 'in_progress'
		  AND (a.teacher_id = ? OR a.substitute_teacher_id = ?)
		LIMIT 1`
	err = config.DB.QueryRow(query, dateStr, roomID, authUserID, authUserID).
		Scan(&attID, &scheduleID, &clockIn, &startTime, &endTime, &targetJP)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Tidak ditemukan presensi aktif untuk discan keluar")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mencari presensi: "+err.Error())
		return
	}

	scheduledStart, _ := parseClockToday(now, startTime)
	scheduledEnd, _ := parseClockToday(now, endTime)

	// Waktu efektif mengajar:
	//   - mulai dari jam JADWAL dimulai (bukan jam scan masuk aktual) -> waktu tunggu tidak dihitung.
	//   - tapi jika guru clock-in setelah jadwal mulai, efektif dimulai dari clock-in aktual.
	effectiveStart := scheduledStart
	if clockIn.After(scheduledStart) {
		effectiveStart = clockIn
	}

	// Batas akhir perhitungan: mana yang lebih cepat antara jadwal selesai vs scan out aktual.
	effectiveEnd := scheduledEnd
	if now.Before(scheduledEnd) {
		effectiveEnd = now
	}

	durationMinutes := effectiveEnd.Sub(effectiveStart).Minutes()
	if durationMinutes < 0 {
		durationMinutes = 0
	}

	jpDuration := getSettingInt("jp_duration_minutes", 45)
	actualJP := math.Floor(durationMinutes / float64(jpDuration))

	status := "tidak_tuntas"
	if int(actualJP) >= targetJP {
		status = "tuntas"
	}

	_, err = config.DB.Exec(
		`UPDATE attendances SET clock_out = ?, actual_jp = ?, status = ? WHERE id = ?`,
		now, actualJP, status, attID,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menyimpan scan keluar: "+err.Error())
		return
	}

	earlyClockOut := now.Before(scheduledEnd)

	utils.Success(w, http.StatusOK, "Scan keluar berhasil dicatat", map[string]interface{}{
		"attendance_id":   attID,
		"clock_out":       now.Format(time.RFC3339),
		"actual_jp":       actualJP,
		"target_jp":       targetJP,
		"status":          status,
		"early_clock_out": earlyClockOut,
	})
}
