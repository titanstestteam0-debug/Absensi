package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"absensi-backend/config"
	"absensi-backend/utils"
)

// =====================================================================
// GET /api/admin/reports/daily?year=2026&month=8&start_day=1
// Laporan absensi harian: rekap per tanggal (bukan per guru) dalam rentang
// [start_day, end_day] pada bulan yang dipilih. Defaultnya start_day = 1
// dan end_day = hari ini (kalau bulan/tahun yang dipilih adalah bulan
// berjalan) atau tanggal terakhir bulan itu (kalau bulan yang dipilih sudah
// lewat). start_day boleh digeser admin, tapi TIDAK BOLEH keluar dari bulan
// yang sama (dibatasi 1..jumlah_hari_di_bulan_itu).
// =====================================================================
func DailyAttendanceReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	year, err1 := strconv.Atoi(q.Get("year"))
	month, err2 := strconv.Atoi(q.Get("month"))
	if err1 != nil || err2 != nil || month < 1 || month > 12 {
		utils.Error(w, http.StatusBadRequest, "Parameter year dan month wajib diisi & valid (contoh: ?year=2026&month=8)")
		return
	}

	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
	daysInMonth := lastOfMonth.Day()

	startDay, err := strconv.Atoi(q.Get("start_day"))
	if err != nil || startDay < 1 {
		startDay = 1
	}
	if startDay > daysInMonth {
		startDay = daysInMonth
	}

	now := time.Now()
	isCurrentMonth := year == now.Year() && int(now.Month()) == month

	endDay := daysInMonth
	if isCurrentMonth {
		endDay = now.Day()
	}
	if startDay > endDay {
		// Admin menggeser start_day melewati batas akhir yang tersedia
		// (mis. memilih tanggal 20 padahal hari ini baru tanggal 10).
		utils.Error(w, http.StatusBadRequest, "Tanggal mulai tidak boleh melebihi tanggal akhir laporan (hari ini, untuk bulan berjalan)")
		return
	}

	startDate := time.Date(year, time.Month(month), startDay, 0, 0, 0, 0, time.Local).Format("2006-01-02")
	endDate := time.Date(year, time.Month(month), endDay, 0, 0, 0, 0, time.Local).Format("2006-01-02")

	rows, err := config.DB.Query(`
		SELECT
			a.date,
			COUNT(*) AS total_sesi,
			SUM(CASE WHEN a.status = 'tuntas' THEN 1 ELSE 0 END) AS sesi_tuntas,
			SUM(CASE WHEN a.status = 'tidak_tuntas' THEN 1 ELSE 0 END) AS sesi_tidak_tuntas,
			SUM(CASE WHEN a.status = 'in_progress' THEN 1 ELSE 0 END) AS sesi_berlangsung,
			COUNT(DISTINCT a.teacher_id) AS jumlah_guru_hadir,
			SUM(CASE WHEN a.substitute_teacher_id IS NOT NULL THEN 1 ELSE 0 END) AS sesi_digantikan,
			IFNULL(SUM(a.actual_jp), 0) AS total_jp_aktual
		FROM attendances a
		WHERE a.date BETWEEN ? AND ?
		GROUP BY a.date
		ORDER BY a.date ASC`,
		startDate, endDate,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal membuat laporan harian: "+err.Error())
		return
	}
	defer rows.Close()

	type dailyRow struct {
		Date            string  `json:"date"`
		TotalSesi       int     `json:"total_sesi"`
		SesiTuntas      int     `json:"sesi_tuntas"`
		SesiTidakTuntas int     `json:"sesi_tidak_tuntas"`
		SesiBerlangsung int     `json:"sesi_berlangsung"`
		JumlahGuruHadir int     `json:"jumlah_guru_hadir"`
		SesiDigantikan  int     `json:"sesi_digantikan"`
		TotalJPAktual   float64 `json:"total_jp_aktual"`
	}

	byDate := map[string]dailyRow{}
	for rows.Next() {
		var d dailyRow
		var dateVal time.Time
		if err := rows.Scan(&dateVal, &d.TotalSesi, &d.SesiTuntas, &d.SesiTidakTuntas,
			&d.SesiBerlangsung, &d.JumlahGuruHadir, &d.SesiDigantikan, &d.TotalJPAktual); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data laporan: "+err.Error())
			return
		}
		d.Date = dateVal.Format("2006-01-02")
		byDate[d.Date] = d
	}

	// Sertakan tanggal yang tidak punya data sekalipun (total_sesi = 0),
	// supaya admin bisa lihat hari mana yang benar-benar kosong aktivitasnya
	// (bukan cuma yang ada datanya saja).
	var report []dailyRow
	for day := startDay; day <= endDay; day++ {
		dateStr := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local).Format("2006-01-02")
		if row, ok := byDate[dateStr]; ok {
			report = append(report, row)
		} else {
			report = append(report, dailyRow{Date: dateStr})
		}
	}

	utils.Success(w, http.StatusOK, "Berhasil membuat laporan absensi harian", map[string]interface{}{
		"year":       year,
		"month":      month,
		"start_day":  startDay,
		"end_day":    endDay,
		"start_date": startDate,
		"end_date":   endDate,
		"days":       report,
	})
}

// =====================================================================
// GET /api/admin/reports/substitutes?start=2026-08-01&end=2026-08-31&teacher_id=5
// Rekap SEMUA sesi di mana seseorang bertindak sebagai GURU PENGGANTI.
// Sengaja TIDAK menampilkan status tuntas/tidak_tuntas maupun target_jp --
// sesi inval tidak terikat target JP guru utama, jadi cukup ditampilkan
// sebagai jumlah sesi & jam aktual mengajar sebagai pengganti.
// teacher_id (opsional) memfilter berdasarkan SIAPA yang jadi pengganti.
// =====================================================================
func SubstituteSessionsReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	start := q.Get("start")
	end := q.Get("end")
	teacherID := q.Get("teacher_id")

	query := `
		SELECT a.id, a.date, sub.id AS substitute_id, sub.name AS substitute_name,
		       orig.id AS original_teacher_id, orig.name AS original_teacher_name,
		       rm.name AS room_name, a.clock_in, a.clock_out, a.actual_jp
		FROM attendances a
		JOIN users sub ON sub.id = a.substitute_teacher_id
		JOIN users orig ON orig.id = a.teacher_id
		LEFT JOIN rooms rm ON rm.id = a.room_id_scanned
		WHERE a.substitute_teacher_id IS NOT NULL`
	args := []interface{}{}

	if start != "" {
		query += " AND a.date >= ?"
		args = append(args, start)
	}
	if end != "" {
		query += " AND a.date <= ?"
		args = append(args, end)
	}
	if teacherID != "" {
		query += " AND a.substitute_teacher_id = ?"
		args = append(args, teacherID)
	}
	query += " ORDER BY a.date DESC, a.clock_in DESC"

	rows, err := config.DB.Query(query, args...)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil rekap guru pengganti: "+err.Error())
		return
	}
	defer rows.Close()

	type substituteRow struct {
		ID                  uint64  `json:"id"`
		Date                string  `json:"date"`
		SubstituteID        uint64  `json:"substitute_id"`
		SubstituteName      string  `json:"substitute_name"`
		OriginalTeacherID   uint64  `json:"original_teacher_id"`
		OriginalTeacherName string  `json:"original_teacher_name"`
		RoomName            *string `json:"room_name"`
		ClockIn             *string `json:"clock_in"`
		ClockOut            *string `json:"clock_out"`
		ActualJP            float64 `json:"actual_jp"`
	}

	var list []substituteRow
	var totalJP float64
	for rows.Next() {
		var s substituteRow
		var clockIn, clockOut sql.NullTime
		var roomName sql.NullString
		if err := rows.Scan(&s.ID, &s.Date, &s.SubstituteID, &s.SubstituteName,
			&s.OriginalTeacherID, &s.OriginalTeacherName, &roomName, &clockIn, &clockOut, &s.ActualJP); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		if roomName.Valid {
			s.RoomName = &roomName.String
		}
		if clockIn.Valid {
			v := clockIn.Time.Format("2006-01-02 15:04:05")
			s.ClockIn = &v
		}
		if clockOut.Valid {
			v := clockOut.Time.Format("2006-01-02 15:04:05")
			s.ClockOut = &v
		}
		totalJP += s.ActualJP
		list = append(list, s)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil rekap sesi guru pengganti", map[string]interface{}{
		"total_sesi": len(list),
		"total_jp":   totalJP,
		"sessions":   list,
	})
}

// =====================================================================
// GET /api/admin/reports/annual?year=2026&teacher_id=5
// Rekap tahunan per guru: breakdown 12 bulan (jadwal reguler vs sebagai
// pengganti vs jumlah cuti), plus indikator "konsisten" per bulan (guru
// tidak pernah tidak_tuntas di bulan tsb) dan ringkasan total setahun.
// Dipakai juga sebagai basis export "Laporan CSV Individu" di frontend.
// =====================================================================
func AnnualTeacherReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	year, errYear := strconv.Atoi(q.Get("year"))
	teacherIDStr := q.Get("teacher_id")
	if errYear != nil || teacherIDStr == "" {
		utils.Error(w, http.StatusBadRequest, "Parameter year dan teacher_id wajib diisi (contoh: ?year=2026&teacher_id=5)")
		return
	}
	teacherID, err := strconv.ParseUint(teacherIDStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "teacher_id tidak valid")
		return
	}

	var teacherName, teacherRole string
	err = config.DB.QueryRow(`SELECT name, role FROM users WHERE id = ?`, teacherID).Scan(&teacherName, &teacherRole)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Guru tidak ditemukan")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data guru: "+err.Error())
		return
	}

	// --- 1) Sesi sesuai jadwal tetap guru ini (teacher_id = jadwal miliknya) ---
	type monthAgg struct {
		TotalSesi       int
		SesiTuntas      int
		SesiTidakTuntas int
		JPAktual        float64
		JPTarget        float64
	}
	scheduled := map[int]*monthAgg{}
	rows, err := config.DB.Query(`
		SELECT MONTH(a.date) AS bln,
		       COUNT(a.id) AS total_sesi,
		       SUM(CASE WHEN a.status = 'tuntas' THEN 1 ELSE 0 END) AS sesi_tuntas,
		       SUM(CASE WHEN a.status = 'tidak_tuntas' THEN 1 ELSE 0 END) AS sesi_tidak_tuntas,
		       IFNULL(SUM(a.actual_jp), 0) AS jp_aktual,
		       IFNULL(SUM(s.target_jp), 0) AS jp_target
		FROM attendances a
		LEFT JOIN schedules s ON s.id = a.schedule_id
		WHERE a.teacher_id = ? AND YEAR(a.date) = ?
		GROUP BY MONTH(a.date)`, teacherID, year)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil rekap jadwal tetap: "+err.Error())
		return
	}
	for rows.Next() {
		var bln int
		agg := &monthAgg{}
		if err := rows.Scan(&bln, &agg.TotalSesi, &agg.SesiTuntas, &agg.SesiTidakTuntas, &agg.JPAktual, &agg.JPTarget); err != nil {
			rows.Close()
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		scheduled[bln] = agg
	}
	rows.Close()

	// --- 2) Sesi sebagai GURU PENGGANTI (tidak terikat target JP apapun) ---
	type subAgg struct {
		SesiSebagaiPengganti int
		JPSebagaiPengganti   float64
	}
	substituted := map[int]*subAgg{}
	rows2, err := config.DB.Query(`
		SELECT MONTH(a.date) AS bln, COUNT(*) AS sesi, IFNULL(SUM(a.actual_jp), 0) AS jp
		FROM attendances a
		WHERE a.substitute_teacher_id = ? AND YEAR(a.date) = ?
		GROUP BY MONTH(a.date)`, teacherID, year)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil rekap sebagai guru pengganti: "+err.Error())
		return
	}
	for rows2.Next() {
		var bln int
		agg := &subAgg{}
		if err := rows2.Scan(&bln, &agg.SesiSebagaiPengganti, &agg.JPSebagaiPengganti); err != nil {
			rows2.Close()
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		substituted[bln] = agg
	}
	rows2.Close()

	// --- 3) Cuti disetujui yang bersinggungan dengan tahun ini, dihitung per bulan ---
	type leaveRange struct{ start, end time.Time }
	var leaveRanges []leaveRange
	yearStart := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
	yearEnd := time.Date(year, 12, 31, 0, 0, 0, 0, time.Local)
	rows3, err := config.DB.Query(`
		SELECT start_date, end_date FROM leaves
		WHERE teacher_id = ? AND status = 'approved'
		  AND start_date <= ? AND end_date >= ?`,
		teacherID, yearEnd.Format("2006-01-02"), yearStart.Format("2006-01-02"))
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data cuti: "+err.Error())
		return
	}
	for rows3.Next() {
		var s, e time.Time
		if err := rows3.Scan(&s, &e); err != nil {
			rows3.Close()
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data cuti: "+err.Error())
			return
		}
		leaveRanges = append(leaveRanges, leaveRange{s, e})
	}
	rows3.Close()

	monthNames := []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember"}

	type monthRow struct {
		Month                int     `json:"month"`
		MonthName            string  `json:"month_name"`
		TotalSesi            int     `json:"total_sesi"`
		SesiTuntas           int     `json:"sesi_tuntas"`
		SesiTidakTuntas      int     `json:"sesi_tidak_tuntas"`
		JPAktual             float64 `json:"jp_aktual"`
		JPTarget             float64 `json:"jp_target"`
		SesiSebagaiPengganti int     `json:"sesi_sebagai_pengganti"`
		JPSebagaiPengganti   float64 `json:"jp_sebagai_pengganti"`
		JumlahCuti           int     `json:"jumlah_cuti"`
		HasData              bool    `json:"has_data"`
		Consistent           bool    `json:"consistent"`
	}

	months := make([]monthRow, 0, 12)
	var totalSesi, totalTuntas, totalTidakTuntas, totalSesiPengganti, totalCuti int
	var totalJPAktual, totalJPTarget, totalJPPengganti float64

	for m := 1; m <= 12; m++ {
		mr := monthRow{Month: m, MonthName: monthNames[m-1]}
		if agg, ok := scheduled[m]; ok {
			mr.TotalSesi = agg.TotalSesi
			mr.SesiTuntas = agg.SesiTuntas
			mr.SesiTidakTuntas = agg.SesiTidakTuntas
			mr.JPAktual = agg.JPAktual
			mr.JPTarget = agg.JPTarget
			mr.HasData = agg.TotalSesi > 0
		}
		if sub, ok := substituted[m]; ok {
			mr.SesiSebagaiPengganti = sub.SesiSebagaiPengganti
			mr.JPSebagaiPengganti = sub.JPSebagaiPengganti
			if sub.SesiSebagaiPengganti > 0 {
				mr.HasData = true
			}
		}

		monthStart := time.Date(year, time.Month(m), 1, 0, 0, 0, 0, time.Local)
		monthEnd := monthStart.AddDate(0, 1, -1)
		for _, lr := range leaveRanges {
			if lr.start.Before(monthEnd.AddDate(0, 0, 1)) && lr.end.After(monthStart.AddDate(0, 0, -1)) {
				mr.JumlahCuti++
			}
		}

		// "Konsisten" = tidak pernah tidak_tuntas di bulan itu (hanya relevan
		// kalau memang ada sesi sesuai jadwal tetap; guru pengganti murni
		// -- yang cuma tampil di kolom sebagai-pengganti -- selalu dianggap
		// konsisten karena memang tidak terikat target JP apapun).
		mr.Consistent = mr.SesiTidakTuntas == 0

		totalSesi += mr.TotalSesi
		totalTuntas += mr.SesiTuntas
		totalTidakTuntas += mr.SesiTidakTuntas
		totalJPAktual += mr.JPAktual
		totalJPTarget += mr.JPTarget
		totalSesiPengganti += mr.SesiSebagaiPengganti
		totalJPPengganti += mr.JPSebagaiPengganti
		totalCuti += mr.JumlahCuti

		months = append(months, mr)
	}

	utils.Success(w, http.StatusOK, "Berhasil membuat rekap tahunan", map[string]interface{}{
		"teacher_id":                    teacherID,
		"teacher_name":                  teacherName,
		"teacher_role":                  teacherRole,
		"year":                          year,
		"months":                        months,
		"total_sesi":                    totalSesi,
		"total_sesi_tuntas":             totalTuntas,
		"total_sesi_tidak_tuntas":       totalTidakTuntas,
		"total_jp_aktual":               totalJPAktual,
		"total_jp_target":               totalJPTarget,
		"total_sesi_sebagai_pengganti":  totalSesiPengganti,
		"total_jp_sebagai_pengganti":    totalJPPengganti,
		"total_jumlah_cuti":             totalCuti,
		"konsisten_setahun":             totalTidakTuntas == 0,
	})
}
