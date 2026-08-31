package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"absensi-backend/config"
	"absensi-backend/models"
	"absensi-backend/utils"

	"github.com/gorilla/mux"
)

// LeaveTypeRequest dipakai untuk create & update master data jenis cuti/izin.
type LeaveTypeRequest struct {
	Code  string `json:"code"`  // nilai yang disimpan di leaves.leave_type, mis. "sakit"
	Label string `json:"label"` // teks yang ditampilkan ke pengguna, mis. "Sakit"
}

func normalizeLeaveTypeCode(code string) string {
	code = strings.TrimSpace(strings.ToLower(code))
	code = strings.ReplaceAll(code, " ", "_")
	return code
}

// GET /api/admin/leave-types
// Admin melihat SEMUA jenis cuti/izin (termasuk yang sudah dinonaktifkan)
// supaya bisa dikelola lewat dashboard, tanpa perlu hardcode di kode frontend.
func ListLeaveTypes(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(
		`SELECT id, code, label, is_active, created_at FROM leave_types ORDER BY label ASC`,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data jenis cuti/izin: "+err.Error())
		return
	}
	defer rows.Close()

	var types []models.LeaveType
	for rows.Next() {
		var lt models.LeaveType
		if err := rows.Scan(&lt.ID, &lt.Code, &lt.Label, &lt.IsActive, &lt.CreatedAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		types = append(types, lt)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil data jenis cuti/izin", types)
}

// GET /api/leave-types
// Guru hanya melihat jenis cuti/izin yang AKTIF, dipakai untuk mengisi
// dropdown di form pengajuan cuti (bukan lagi daftar hardcode di frontend).
func ListActiveLeaveTypes(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(
		`SELECT id, code, label, is_active, created_at FROM leave_types WHERE is_active = 1 ORDER BY label ASC`,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data jenis cuti/izin: "+err.Error())
		return
	}
	defer rows.Close()

	var types []models.LeaveType
	for rows.Next() {
		var lt models.LeaveType
		if err := rows.Scan(&lt.ID, &lt.Code, &lt.Label, &lt.IsActive, &lt.CreatedAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		types = append(types, lt)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil data jenis cuti/izin", types)
}

// POST /api/admin/leave-types
func CreateLeaveType(w http.ResponseWriter, r *http.Request) {
	var req LeaveTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	if req.Label == "" {
		utils.Error(w, http.StatusBadRequest, "label wajib diisi")
		return
	}
	code := normalizeLeaveTypeCode(req.Code)
	if code == "" {
		code = normalizeLeaveTypeCode(req.Label)
	}
	if code == "" {
		utils.Error(w, http.StatusBadRequest, "code/label wajib diisi")
		return
	}

	result, err := config.DB.Exec(
		`INSERT INTO leave_types (code, label) VALUES (?, ?)`,
		code, req.Label,
	)
	if err != nil {
		utils.Error(w, http.StatusConflict, "Gagal membuat jenis cuti/izin (kode mungkin sudah dipakai): "+err.Error())
		return
	}

	id, _ := result.LastInsertId()
	utils.Success(w, http.StatusCreated, "Jenis cuti/izin berhasil ditambahkan", map[string]interface{}{
		"id":    id,
		"code":  code,
		"label": req.Label,
	})
}

// PUT /api/admin/leave-types/{id}
// Hanya label yang bisa diubah -- code sengaja tidak diizinkan berubah supaya
// data leaves lama yang sudah memakai code tersebut tidak jadi tidak sinkron.
func UpdateLeaveType(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var req LeaveTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	if req.Label == "" {
		utils.Error(w, http.StatusBadRequest, "label wajib diisi")
		return
	}

	if _, err := config.DB.Exec(`UPDATE leave_types SET label = ? WHERE id = ?`, req.Label, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memperbarui jenis cuti/izin: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Jenis cuti/izin berhasil diperbarui", nil)
}

// DELETE /api/admin/leave-types/{id}
// Soft-delete (nonaktifkan) supaya riwayat pengajuan cuti lama yang masih
// memakai jenis ini tetap bisa ditampilkan dengan benar.
func DeleteLeaveType(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	if _, err := config.DB.Exec(`UPDATE leave_types SET is_active = 0 WHERE id = ?`, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menonaktifkan jenis cuti/izin: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Jenis cuti/izin berhasil dinonaktifkan", nil)
}

// PUT /api/admin/leave-types/{id}/activate
func ActivateLeaveType(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	if _, err := config.DB.Exec(`UPDATE leave_types SET is_active = 1 WHERE id = ?`, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengaktifkan jenis cuti/izin: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Jenis cuti/izin berhasil diaktifkan", nil)
}
