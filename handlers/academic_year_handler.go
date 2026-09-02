package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"absensi-backend/config"
	"absensi-backend/models"
	"absensi-backend/utils"

	"github.com/gorilla/mux"
)

type AcademicYearRequest struct {
	StartYear int `json:"start_year"` // label & end_year dihitung otomatis dari ini
}

// GET /api/admin/academic-years
func ListAcademicYears(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(
		`SELECT id, label, start_year, end_year, is_active FROM academic_years ORDER BY start_year DESC`,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data tahun ajaran: "+err.Error())
		return
	}
	defer rows.Close()

	var years []models.AcademicYear
	for rows.Next() {
		var y models.AcademicYear
		if err := rows.Scan(&y.ID, &y.Label, &y.StartYear, &y.EndYear, &y.IsActive); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		years = append(years, y)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil data tahun ajaran", years)
}

// POST /api/admin/academic-years
// Membuat tahun ajaran baru sebagai DRAFT (is_active=0) -- misalnya
// menyiapkan tahun ajaran depan sementara tahun berjalan masih aktif.
// Admin mengaktifkannya belakangan lewat endpoint activate kalau sudah
// waktunya, dan itu otomatis menonaktifkan tahun ajaran lain.
func CreateAcademicYear(w http.ResponseWriter, r *http.Request) {
	var req AcademicYearRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if req.StartYear < 2000 || req.StartYear > 2100 {
		utils.Error(w, http.StatusBadRequest, "start_year tidak valid")
		return
	}

	label := fmt.Sprintf("%d/%d", req.StartYear, req.StartYear+1)
	result, err := config.DB.Exec(
		`INSERT INTO academic_years (label, start_year, end_year, is_active) VALUES (?, ?, ?, 0)`,
		label, req.StartYear, req.StartYear+1,
	)
	if err != nil {
		utils.Error(w, http.StatusConflict, "Gagal membuat tahun ajaran (mungkin sudah ada): "+err.Error())
		return
	}

	id, _ := result.LastInsertId()
	utils.Success(w, http.StatusCreated, "Tahun ajaran "+label+" berhasil dibuat sebagai draft", map[string]interface{}{
		"id":    id,
		"label": label,
	})
}

// PUT /api/admin/academic-years/{id}/activate
// Mengaktifkan satu tahun ajaran, otomatis menonaktifkan semua yang lain --
// hanya SATU tahun ajaran yang boleh aktif dalam satu waktu. Tahun ajaran
// lain (mis. tahun lalu/tahun depan) tetap tersimpan sebagai draft dan bisa
// diaktifkan lagi kapan saja.
func ActivateAcademicYear(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var label string
	err = config.DB.QueryRow(`SELECT label FROM academic_years WHERE id = ?`, id).Scan(&label)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Tahun ajaran tidak ditemukan")
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
	if _, err := tx.Exec(`UPDATE academic_years SET is_active = 0 WHERE is_active = 1`); err != nil {
		tx.Rollback()
		utils.Error(w, http.StatusInternalServerError, "Gagal menonaktifkan tahun ajaran sebelumnya: "+err.Error())
		return
	}
	if _, err := tx.Exec(`UPDATE academic_years SET is_active = 1 WHERE id = ?`, id); err != nil {
		tx.Rollback()
		utils.Error(w, http.StatusInternalServerError, "Gagal mengaktifkan tahun ajaran: "+err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menyimpan perubahan: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Tahun ajaran "+label+" sekarang aktif", nil)
}

// DELETE /api/admin/academic-years/{id}
// Hanya boleh menghapus tahun ajaran yang berstatus draft (bukan yang
// sedang aktif), supaya tidak ada momen sistem tanpa tahun ajaran aktif.
func DeleteAcademicYear(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var isActive bool
	err = config.DB.QueryRow(`SELECT is_active FROM academic_years WHERE id = ?`, id).Scan(&isActive)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Tahun ajaran tidak ditemukan")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memeriksa data: "+err.Error())
		return
	}
	if isActive {
		utils.Error(w, http.StatusBadRequest, "Tidak bisa menghapus tahun ajaran yang sedang aktif. Aktifkan tahun ajaran lain terlebih dahulu.")
		return
	}

	if _, err := config.DB.Exec(`DELETE FROM academic_years WHERE id = ?`, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menghapus tahun ajaran: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Draft tahun ajaran berhasil dihapus", nil)
}
