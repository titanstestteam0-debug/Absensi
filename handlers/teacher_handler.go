package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"absensi-backend/config"
	"absensi-backend/models"
	"absensi-backend/utils"

	"github.com/gorilla/mux"
	"golang.org/x/crypto/bcrypt"
)

type TeacherRequest struct {
	Name     string `json:"name"`
	NIP      string `json:"nip"`
	Email    string `json:"email"`
	Password string `json:"password"` // hanya wajib saat create
	Role     string `json:"role"`     // "admin", "guru", atau "guru_pengganti"
}

// isValidRole memastikan role yang dikirim Admin ada di daftar yang dikenal sistem.
func isValidRole(role string) bool {
	switch role {
	case string(models.RoleAdmin), string(models.RoleGuru), string(models.RoleGuruPengganti):
		return true
	default:
		return false
	}
}

// GET /api/admin/teachers
func ListTeachers(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(`SELECT id, name, nip, email, photo_url, role, is_active, created_at
	                               FROM users ORDER BY name ASC`)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data guru: "+err.Error())
		return
	}
	defer rows.Close()

	var teachers []models.User
	for rows.Next() {
		var u models.User
		var email sql.NullString
		var photoURL sql.NullString
		if err := rows.Scan(&u.ID, &u.Name, &u.NIP, &email, &photoURL, &u.Role, &u.IsActive, &u.CreatedAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		u.Email = email.String
		u.PhotoURL = photoURL.String
		teachers = append(teachers, u)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil data guru", teachers)
}

// POST /api/admin/teachers
func CreateTeacher(w http.ResponseWriter, r *http.Request) {
	var req TeacherRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if req.Name == "" || req.NIP == "" || req.Password == "" {
		utils.Error(w, http.StatusBadRequest, "name, nip, dan password wajib diisi")
		return
	}
	if req.Role == "" {
		req.Role = string(models.RoleGuru)
	}
	if !isValidRole(req.Role) {
		utils.Error(w, http.StatusBadRequest, "role harus 'admin', 'guru', atau 'guru_pengganti'")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengenkripsi password")
		return
	}

	result, err := config.DB.Exec(
		`INSERT INTO users (name, nip, email, password_hash, role) VALUES (?, ?, ?, ?, ?)`,
		req.Name, req.NIP, req.Email, string(hash), req.Role,
	)
	if err != nil {
		utils.Error(w, http.StatusConflict, "Gagal membuat guru (NIP/email mungkin sudah terdaftar): "+err.Error())
		return
	}

	id, _ := result.LastInsertId()
	utils.Success(w, http.StatusCreated, "Guru berhasil ditambahkan", map[string]interface{}{"id": id})
}

// PUT /api/admin/teachers/{id}
func UpdateTeacher(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var req TeacherRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if !isValidRole(req.Role) {
		utils.Error(w, http.StatusBadRequest, "role harus 'admin', 'guru', atau 'guru_pengganti'")
		return
	}

	query := `UPDATE users SET name = ?, email = ?, role = ? WHERE id = ?`
	args := []interface{}{req.Name, req.Email, req.Role, id}

	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal mengenkripsi password")
			return
		}
		query = `UPDATE users SET name = ?, email = ?, role = ?, password_hash = ? WHERE id = ?`
		args = []interface{}{req.Name, req.Email, req.Role, string(hash), id}
	}

	if _, err := config.DB.Exec(query, args...); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memperbarui data guru: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Data guru berhasil diperbarui", nil)
}

// DELETE /api/admin/teachers/{id}
func DeleteTeacher(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	if _, err := config.DB.Exec(`UPDATE users SET is_active = 0 WHERE id = ?`, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menonaktifkan guru: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Guru berhasil dinonaktifkan", nil)
}

// PUT /api/admin/teachers/{id}/activate
// Mengaktifkan kembali akun guru yang sebelumnya dinonaktifkan, supaya bisa
// login dan presensi lagi.
func ActivateTeacher(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	if _, err := config.DB.Exec(`UPDATE users SET is_active = 1 WHERE id = ?`, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengaktifkan guru: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Guru berhasil diaktifkan", nil)
}
