package handlers

import (
	"database/sql"
	"net/http"

	"absensi-backend/config"
	"absensi-backend/middleware"
	"absensi-backend/utils"
)

// GET /api/admin/reports/monthly?month=7&year=2026
// Rekap total jam mengajar aktual vs jadwal per guru dalam 1 bulan.
func MonthlyReport(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")
	year := r.URL.Query().Get("year")
	if month == "" || year == "" {
		utils.Error(w, http.StatusBadRequest, "Parameter month dan year wajib diisi (contoh: ?month=7&year=2026)")
		return
	}

	query := `
		SELECT
			u.id AS teacher_id,
			u.name AS teacher_name,
			COUNT(a.id) AS total_sesi,
			SUM(CASE WHEN a.status = 'tuntas' THEN 1 ELSE 0 END) AS sesi_tuntas,
			SUM(CASE WHEN a.status = 'tidak_tuntas' THEN 1 ELSE 0 END) AS sesi_tidak_tuntas,
			IFNULL(SUM(a.actual_jp), 0) AS total_jp_aktual,
			IFNULL(SUM(s.target_jp), 0) AS total_jp_target
		FROM users u
		LEFT JOIN attendances a
			ON a.teacher_id = u.id
			AND MONTH(a.date) = ? AND YEAR(a.date) = ?
		LEFT JOIN schedules s ON s.id = a.schedule_id
		WHERE u.role IN ('guru', 'guru_pengganti')
		GROUP BY u.id, u.name
		ORDER BY u.name ASC`

	rows, err := config.DB.Query(query, month, year)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal membuat laporan bulanan: "+err.Error())
		return
	}
	defer rows.Close()

	type teacherRecap struct {
		TeacherID       uint64  `json:"teacher_id"`
		TeacherName     string  `json:"teacher_name"`
		TotalSesi       int     `json:"total_sesi"`
		SesiTuntas      int     `json:"sesi_tuntas"`
		SesiTidakTuntas int     `json:"sesi_tidak_tuntas"`
		TotalJPAktual   float64 `json:"total_jp_aktual"`
		TotalJPTarget   float64 `json:"total_jp_target"`
	}

	var recap []teacherRecap
	for rows.Next() {
		var t teacherRecap
		if err := rows.Scan(&t.TeacherID, &t.TeacherName, &t.TotalSesi, &t.SesiTuntas,
			&t.SesiTidakTuntas, &t.TotalJPAktual, &t.TotalJPTarget); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data laporan: "+err.Error())
			return
		}
		recap = append(recap, t)
	}

	utils.Success(w, http.StatusOK, "Berhasil membuat laporan bulanan", recap)

	// Catatan untuk Akbar/Daniel: untuk export Excel/PDF, endpoint ini bisa dipanggil
	// lalu di-render di sisi Svelte (mis. pakai SheetJS untuk Excel, atau lib PDF di client),
	// atau tambahkan endpoint terpisah /api/admin/reports/monthly/export yang meng-generate
	// file langsung dari Go (mis. dengan excelize / gofpdf) jika dibutuhkan sisi server.
}

// GET /api/admin/reports/history?teacher_id=2&start=2026-07-01&end=2026-07-31
// History Log: detail waktu scan in/out, ruangan, dan status per guru.
func HistoryLog(w http.ResponseWriter, r *http.Request) {
	teacherID := r.URL.Query().Get("teacher_id")
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")

	query := `
		SELECT a.id, a.date, u.name AS teacher_name, rm.name AS room_name,
		       a.clock_in, a.clock_out, a.actual_jp, s.target_jp, a.status,
		       sub.name AS substitute_name
		FROM attendances a
		JOIN users u ON u.id = a.teacher_id
		JOIN schedules s ON s.id = a.schedule_id
		LEFT JOIN rooms rm ON rm.id = a.room_id_scanned
		LEFT JOIN users sub ON sub.id = a.substitute_teacher_id
		WHERE 1=1`
	args := []interface{}{}

	if teacherID != "" {
		query += " AND a.teacher_id = ?"
		args = append(args, teacherID)
	}
	if start != "" {
		query += " AND a.date >= ?"
		args = append(args, start)
	}
	if end != "" {
		query += " AND a.date <= ?"
		args = append(args, end)
	}
	query += " ORDER BY a.date DESC, a.clock_in DESC"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil history log: "+err.Error())
		return
	}
	defer rows.Close()

	type historyRow struct {
		ID             uint64  `json:"id"`
		Date           string  `json:"date"`
		TeacherName    string  `json:"teacher_name"`
		RoomName       *string `json:"room_name"`
		ClockIn        *string `json:"clock_in"`
		ClockOut       *string `json:"clock_out"`
		ActualJP       float64 `json:"actual_jp"`
		TargetJP       int     `json:"target_jp"`
		Status         string  `json:"status"`
		SubstituteName *string `json:"substitute_name"`
	}

	var history []historyRow
	for rows.Next() {
		var h historyRow
		var clockIn, clockOut sql.NullTime
		var roomName, subName sql.NullString
		if err := rows.Scan(&h.ID, &h.Date, &h.TeacherName, &roomName, &clockIn, &clockOut,
			&h.ActualJP, &h.TargetJP, &h.Status, &subName); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		if roomName.Valid {
			h.RoomName = &roomName.String
		}
		if subName.Valid {
			h.SubstituteName = &subName.String
		}
		if clockIn.Valid {
			s := clockIn.Time.Format("2006-01-02 15:04:05")
			h.ClockIn = &s
		}
		if clockOut.Valid {
			s := clockOut.Time.Format("2006-01-02 15:04:05")
			h.ClockOut = &s
		}
		history = append(history, h)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil history log", history)
}

// GET /api/attendance/history  (Guru melihat riwayat presensi dirinya sendiri)
func MyAttendanceHistory(w http.ResponseWriter, r *http.Request) {
	teacherID := middleware.GetUserID(r)

	rows, err := config.DB.Query(`
		SELECT a.id, a.date, rm.name, a.clock_in, a.clock_out, a.actual_jp, s.target_jp, a.status,
		       CASE WHEN a.substitute_teacher_id = ? THEN orig.name ELSE NULL END AS substituted_for_name
		FROM attendances a
		JOIN schedules s ON s.id = a.schedule_id
		LEFT JOIN rooms rm ON rm.id = a.room_id_scanned
		JOIN users orig ON orig.id = a.teacher_id
		WHERE a.teacher_id = ? OR a.substitute_teacher_id = ?
		ORDER BY a.date DESC, a.clock_in DESC`, teacherID, teacherID, teacherID)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil riwayat: "+err.Error())
		return
	}
	defer rows.Close()

	type myHistoryRow struct {
		ID                 uint64  `json:"id"`
		Date               string  `json:"date"`
		RoomName           *string `json:"room_name"`
		ClockIn            *string `json:"clock_in"`
		ClockOut           *string `json:"clock_out"`
		ActualJP           float64 `json:"actual_jp"`
		TargetJP           int     `json:"target_jp"`
		Status             string  `json:"status"`
		SubstitutedForName *string `json:"substituted_for_name"`
	}

	var history []myHistoryRow
	for rows.Next() {
		var h myHistoryRow
		var clockIn, clockOut sql.NullTime
		var roomName, substitutedFor sql.NullString
		if err := rows.Scan(&h.ID, &h.Date, &roomName, &clockIn, &clockOut, &h.ActualJP, &h.TargetJP, &h.Status, &substitutedFor); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		if roomName.Valid {
			h.RoomName = &roomName.String
		}
		if substitutedFor.Valid {
			h.SubstitutedForName = &substitutedFor.String
		}
		if clockIn.Valid {
			s := clockIn.Time.Format("2006-01-02 15:04:05")
			h.ClockIn = &s
		}
		if clockOut.Valid {
			s := clockOut.Time.Format("2006-01-02 15:04:05")
			h.ClockOut = &s
		}
		history = append(history, h)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil riwayat presensi", history)
}
