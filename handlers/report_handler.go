package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"absensi-backend/config"
	"absensi-backend/middleware"
	"absensi-backend/utils"
)

// monthBounds mengembalikan tanggal pertama & terakhir (format YYYY-MM-DD)
// dari sebuah bulan-tahun, dipakai bersama oleh beberapa endpoint laporan
// supaya perhitungan rentang tanggal konsisten (pakai time.Date, bukan
// string manipulation, supaya otomatis benar untuk bulan 28/29/30/31 hari).
func monthBounds(year, month int) (firstDay string, lastDay string) {
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	last := first.AddDate(0, 1, -1)
	return first.Format("2006-01-02"), last.Format("2006-01-02")
}

// GET /api/admin/reports/monthly?month=7&year=2026
// Rekap total jam mengajar aktual vs jadwal per guru dalam 1 bulan, plus
// info seberapa sering guru tsb bertindak sebagai/mempunyai guru pengganti,
// dan berapa kali ia mengajukan cuti pada bulan tsb.
func MonthlyReport(w http.ResponseWriter, r *http.Request) {
	monthStr := r.URL.Query().Get("month")
	yearStr := r.URL.Query().Get("year")
	if monthStr == "" || yearStr == "" {
		utils.Error(w, http.StatusBadRequest, "Parameter month dan year wajib diisi (contoh: ?month=7&year=2026)")
		return
	}
	month, err1 := strconv.Atoi(monthStr)
	year, err2 := strconv.Atoi(yearStr)
	if err1 != nil || err2 != nil || month < 1 || month > 12 {
		utils.Error(w, http.StatusBadRequest, "Parameter month/year tidak valid")
		return
	}
	firstDay, lastDay := monthBounds(year, month)

	query := `
		SELECT
			u.id AS teacher_id,
			u.name AS teacher_name,
			u.role AS teacher_role,
			COUNT(a.id) AS total_sesi,
			SUM(CASE WHEN a.status = 'tuntas' THEN 1 ELSE 0 END) AS sesi_tuntas,
			SUM(CASE WHEN a.status = 'tidak_tuntas' THEN 1 ELSE 0 END) AS sesi_tidak_tuntas,
			IFNULL(SUM(a.actual_jp), 0) AS total_jp_aktual,
			IFNULL(SUM(s.target_jp), 0) AS total_jp_target,
			-- Berapa kali guru ini TAMPIL SEBAGAI GURU PENGGANTI (menggantikan
			-- guru lain) bulan ini. Sengaja TIDAK dibandingkan dengan target JP
			-- apapun -- sesi inval bersifat ad-hoc, bukan jadwal tetap dia.
			(SELECT COUNT(*) FROM attendances a2
			   WHERE a2.substitute_teacher_id = u.id AND a2.date BETWEEN ? AND ?) AS sesi_sebagai_pengganti,
			(SELECT IFNULL(SUM(a2.actual_jp), 0) FROM attendances a2
			   WHERE a2.substitute_teacher_id = u.id AND a2.date BETWEEN ? AND ?) AS jp_sebagai_pengganti,
			-- Berapa kali jadwal guru ini DIGANTIKAN oleh guru pengganti bulan ini.
			(SELECT COUNT(*) FROM attendances a3
			   WHERE a3.teacher_id = u.id AND a3.substitute_teacher_id IS NOT NULL
			     AND a3.date BETWEEN ? AND ?) AS sesi_digantikan,
			-- Berapa kali pengajuan cuti (disetujui) yang bersinggungan dengan bulan ini.
			(SELECT COUNT(*) FROM leaves l
			   WHERE l.teacher_id = u.id AND l.status = 'approved'
			     AND l.start_date <= ? AND l.end_date >= ?) AS jumlah_cuti
		FROM users u
		LEFT JOIN attendances a
			ON a.teacher_id = u.id
			AND a.date BETWEEN ? AND ?
		LEFT JOIN schedules s ON s.id = a.schedule_id
		WHERE u.role IN ('guru', 'guru_pengganti')
		GROUP BY u.id, u.name, u.role
		ORDER BY u.name ASC`

	rows, err := config.DB.Query(query,
		firstDay, lastDay, // sesi_sebagai_pengganti
		firstDay, lastDay, // jp_sebagai_pengganti
		firstDay, lastDay, // sesi_digantikan
		lastDay, firstDay, // jumlah_cuti (overlap: start<=lastDay AND end>=firstDay)
		firstDay, lastDay, // JOIN attendances utama
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal membuat laporan bulanan: "+err.Error())
		return
	}
	defer rows.Close()

	type teacherRecap struct {
		TeacherID            uint64  `json:"teacher_id"`
		TeacherName          string  `json:"teacher_name"`
		TeacherRole          string  `json:"teacher_role"`
		TotalSesi            int     `json:"total_sesi"`
		SesiTuntas           int     `json:"sesi_tuntas"`
		SesiTidakTuntas      int     `json:"sesi_tidak_tuntas"`
		TotalJPAktual        float64 `json:"total_jp_aktual"`
		TotalJPTarget        float64 `json:"total_jp_target"`
		SesiSebagaiPengganti int     `json:"sesi_sebagai_pengganti"`
		JPSebagaiPengganti   float64 `json:"jp_sebagai_pengganti"`
		SesiDigantikan       int     `json:"sesi_digantikan"`
		JumlahCuti           int     `json:"jumlah_cuti"`
	}

	var recap []teacherRecap
	for rows.Next() {
		var t teacherRecap
		if err := rows.Scan(&t.TeacherID, &t.TeacherName, &t.TeacherRole, &t.TotalSesi, &t.SesiTuntas,
			&t.SesiTidakTuntas, &t.TotalJPAktual, &t.TotalJPTarget,
			&t.SesiSebagaiPengganti, &t.JPSebagaiPengganti, &t.SesiDigantikan, &t.JumlahCuti); err != nil {
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
