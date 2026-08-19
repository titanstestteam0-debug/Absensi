package middleware

import (
	"context"
	"net/http"
	"strings"

	"absensi-backend/utils"
)

type contextKey string

const (
	ContextUserID contextKey = "user_id"
	ContextRole   contextKey = "role"
)

// JWTAuth memvalidasi header "Authorization: Bearer <token>" dan menyisipkan
// user_id & role ke dalam request context untuk dipakai handler berikutnya.
func JWTAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			utils.Error(w, http.StatusUnauthorized, "Header Authorization tidak ditemukan atau format salah")
			return
		}
		tokenString := strings.TrimPrefix(header, "Bearer ")

		claims, err := utils.ParseToken(tokenString)
		if err != nil {
			utils.Error(w, http.StatusUnauthorized, "Token tidak valid atau kadaluarsa")
			return
		}

		ctx := context.WithValue(r.Context(), ContextUserID, claims.UserID)
		ctx = context.WithValue(ctx, ContextRole, claims.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole membatasi akses endpoint hanya untuk role tertentu (mis. "admin").
// Harus dipasang SETELAH JWTAuth di chain middleware.
func RequireRole(role string) func(http.Handler) http.Handler {
	return RequireAnyRole(role)
}

// RequireAnyRole membatasi akses endpoint untuk beberapa role sekaligus, mis.
// RequireAnyRole("guru", "guru_pengganti") supaya guru utama maupun guru
// pengganti sama-sama bisa presensi & mengajukan cuti, tapi admin murni
// (yang tidak punya jadwal mengajar) tetap tidak bisa. Harus dipasang
// SETELAH JWTAuth di chain middleware.
func RequireAnyRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userRole, ok := r.Context().Value(ContextRole).(string)
			if !ok || !allowed[userRole] {
				utils.Error(w, http.StatusForbidden, "Anda tidak memiliki akses untuk resource ini")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func GetUserID(r *http.Request) uint64 {
	if v, ok := r.Context().Value(ContextUserID).(uint64); ok {
		return v
	}
	return 0
}

func GetRole(r *http.Request) string {
	if v, ok := r.Context().Value(ContextRole).(string); ok {
		return v
	}
	return ""
}
