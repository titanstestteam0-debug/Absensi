package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"absensi-backend/config"
	"absensi-backend/models"
	"absensi-backend/utils"

	"github.com/gorilla/mux"
)

type RoomRequest struct {
	Name string `json:"name"`
}

// generateQRString membuat unique hash pendek untuk QR Code ruangan.
// Contoh hasil: ROOM-K3F9X2P7
func generateQRString() (string, error) {
	buf := make([]byte, 5)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	code := strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))
	return "ROOM-" + code, nil
}

// generateUniqueQRString mencoba beberapa kali membuat qr_string yang belum
// dipakai ruangan lain (kemungkinan tabrakan sangat kecil, tapi tetap dicek).
func generateUniqueQRString() (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		candidate, err := generateQRString()
		if err != nil {
			return "", err
		}
		var exists int
		config.DB.QueryRow(`SELECT COUNT(*) FROM rooms WHERE qr_string = ?`, candidate).Scan(&exists)
		if exists == 0 {
			return candidate, nil
		}
	}
	return "", sql.ErrNoRows
}

// qrRotationSeconds membaca interval otomatis QR berubah dari tabel settings
// (default 20 detik jika belum diatur). Interval yang pendek membuat QR yang
// difoto/discreenshot dari rumah cepat kedaluwarsa dan tidak bisa dipakai lagi.
func qrRotationSeconds() int {
	return getSettingInt("qr_rotation_seconds", 20)
}

// rotateRoomQR membuat qr_string baru untuk sebuah ruangan dan menyimpannya
// beserta waktu kedaluwarsanya. Dipanggil baik oleh refresh manual (admin
// klik tombol) maupun otomatis (lazy-rotate saat qr_string lama sudah expired).
func rotateRoomQR(roomID uint64) (string, time.Time, error) {
	qr, err := generateUniqueQRString()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(time.Duration(qrRotationSeconds()) * time.Second)

	_, err = config.DB.Exec(
		`UPDATE rooms SET qr_string = ?, qr_expires_at = ?, qr_last_rotated_at = NOW() WHERE id = ?`,
		qr, expiresAt, roomID,
	)
	if err != nil {
		return "", time.Time{}, err
	}
	return qr, expiresAt, nil
}

// GET /api/admin/rooms
func ListRooms(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(`SELECT id, name, qr_string, is_active, qr_expires_at FROM rooms ORDER BY name ASC`)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data ruangan: "+err.Error())
		return
	}
	defer rows.Close()

	var rooms []models.Room
	for rows.Next() {
		var rm models.Room
		var expiresAt sql.NullTime
		if err := rows.Scan(&rm.ID, &rm.Name, &rm.QRString, &rm.IsActive, &expiresAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		if expiresAt.Valid {
			t := expiresAt.Time
			rm.QRExpiresAt = &t
		}
		rooms = append(rooms, rm)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil data ruangan", rooms)
}

// POST /api/admin/rooms
// Membuat ruangan baru sekaligus generate qr_string unik (dengan masa
// berlaku awal) untuk langsung ditampilkan sebagai QR live pertama kalinya.
func CreateRoom(w http.ResponseWriter, r *http.Request) {
	var req RoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if req.Name == "" {
		utils.Error(w, http.StatusBadRequest, "name wajib diisi")
		return
	}

	qr, err := generateUniqueQRString()
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal membuat QR string unik, coba lagi")
		return
	}
	expiresAt := time.Now().Add(time.Duration(qrRotationSeconds()) * time.Second)

	result, err := config.DB.Exec(
		`INSERT INTO rooms (name, qr_string, qr_expires_at, qr_last_rotated_at) VALUES (?, ?, ?, NOW())`,
		req.Name, qr, expiresAt,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal membuat ruangan: "+err.Error())
		return
	}

	id, _ := result.LastInsertId()
	utils.Success(w, http.StatusCreated, "Ruangan berhasil dibuat", map[string]interface{}{
		"id":                 id,
		"name":               req.Name,
		"qr_string":          qr,
		"expires_at":         expiresAt.Format(time.RFC3339),
		"expires_in_seconds": qrRotationSeconds(),
	})
}

// PUT /api/admin/rooms/{id}
func UpdateRoom(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var req RoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}

	if _, err := config.DB.Exec(`UPDATE rooms SET name = ? WHERE id = ?`, req.Name, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memperbarui ruangan: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Ruangan berhasil diperbarui", nil)
}

// DELETE /api/admin/rooms/{id}
func DeleteRoom(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	if _, err := config.DB.Exec(`UPDATE rooms SET is_active = 0 WHERE id = ?`, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menonaktifkan ruangan: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Ruangan berhasil dinonaktifkan", nil)
}

// =====================================================================
// GET /api/admin/rooms/{id}/qr
// Dipanggil berulang (polling) oleh frontend selagi modal QR terbuka, untuk
// menampilkan kode QR yang selalu "hidup". Jika qr_string yang tersimpan
// sudah lewat masa berlakunya, endpoint ini otomatis merotasinya (lazy
// rotation) sebelum mengembalikan nilai terbaru -> QR di layar admin selalu
// berubah tanpa perlu proses/cron terpisah di background.
// =====================================================================
func GetRoomQR(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var name, qr string
	var isActive bool
	var expiresAt sql.NullTime
	err = config.DB.QueryRow(`SELECT name, qr_string, is_active, qr_expires_at FROM rooms WHERE id = ?`, id).
		Scan(&name, &qr, &isActive, &expiresAt)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Ruangan tidak ditemukan")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil QR ruangan: "+err.Error())
		return
	}

	now := time.Now()
	expiresAtTime := now // dianggap sudah expired jika belum pernah di-set sebelumnya
	if expiresAt.Valid {
		expiresAtTime = expiresAt.Time
	}

	if !expiresAt.Valid || !now.Before(expiresAtTime) {
		newQR, newExpiresAt, rerr := rotateRoomQR(id)
		if rerr != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal merotasi QR ruangan: "+rerr.Error())
			return
		}
		qr = newQR
		expiresAtTime = newExpiresAt
	}

	expiresIn := int(time.Until(expiresAtTime).Seconds())
	if expiresIn < 0 {
		expiresIn = 0
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil QR ruangan", map[string]interface{}{
		"id":                 id,
		"name":               name,
		"qr_string":          qr,
		"is_active":          isActive,
		"expires_at":         expiresAtTime.Format(time.RFC3339),
		"expires_in_seconds": expiresIn,
		"rotation_seconds":   qrRotationSeconds(),
	})
}

// =====================================================================
// POST /api/admin/rooms/{id}/refresh-qr
// Refresh manual: admin bisa langsung memaksa QR berganti saat itu juga
// (misalnya kalau curiga QR lama sudah difoto/tersebar), tanpa menunggu
// masa berlaku otomatis habis.
// =====================================================================
func RefreshRoomQR(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var name string
	var isActive bool
	err = config.DB.QueryRow(`SELECT name, is_active FROM rooms WHERE id = ?`, id).Scan(&name, &isActive)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Ruangan tidak ditemukan")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data ruangan: "+err.Error())
		return
	}

	qr, expiresAt, err := rotateRoomQR(id)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal me-refresh QR ruangan: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "QR Code berhasil di-refresh", map[string]interface{}{
		"id":                 id,
		"name":               name,
		"qr_string":          qr,
		"is_active":          isActive,
		"expires_at":         expiresAt.Format(time.RFC3339),
		"expires_in_seconds": qrRotationSeconds(),
		"rotation_seconds":   qrRotationSeconds(),
	})
}
