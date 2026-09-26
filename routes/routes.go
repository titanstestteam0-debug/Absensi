package routes

import (
	"net/http"

	"absensi-backend/handlers"
	"absensi-backend/middleware"

	"github.com/gorilla/mux"
)

func SetupRouter() *mux.Router {
	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()

	// -------------------- Public --------------------
	api.HandleFunc("/auth/login", handlers.Login).Methods(http.MethodPost)
	// Identitas sekolah (nama & logo) dipakai buat render header & modal
	// login SEBELUM user login, jadi sengaja tidak diwajibkan JWT.
	api.HandleFunc("/settings/school", handlers.GetSchoolSettings).Methods(http.MethodGet)

	// -------------------- Profil (butuh login, semua role) --------------------
	// Setiap user yang login (admin, guru, guru_pengganti) bisa lihat & update
	// foto profilnya sendiri lewat endpoint ini.
	me := api.PathPrefix("").Subrouter()
	me.Use(middleware.JWTAuth)
	me.Use(middleware.RequireAnyRole("admin", "guru", "guru_pengganti"))

	me.HandleFunc("/profile", handlers.GetProfile).Methods(http.MethodGet)
	me.HandleFunc("/profile/photo", handlers.UpdateProfilePhoto).Methods(http.MethodPut)

	// Notifikasi milik user yang login (lonceng di header): lihat daftar,
	// tandai satu / semua sebagai sudah dibaca. Berlaku untuk semua role.
	me.HandleFunc("/notifications", handlers.ListMyNotifications).Methods(http.MethodGet)
	me.HandleFunc("/notifications/read-all", handlers.MarkAllNotificationsRead).Methods(http.MethodPut)
	me.HandleFunc("/notifications/{id}/read", handlers.MarkNotificationRead).Methods(http.MethodPut)

	// -------------------- Guru & Guru Pengganti (butuh login) --------------------
	// Guru Pengganti (Inval) memakai endpoint yang sama persis dengan Guru
	// utama untuk presensi (dengan substitute_for_id) dan pengajuan cuti,
	// jadi kedua role diizinkan lewat RequireAnyRole di sini.
	guru := api.PathPrefix("").Subrouter()
	guru.Use(middleware.JWTAuth)
	guru.Use(middleware.RequireAnyRole("guru", "guru_pengganti"))

	guru.HandleFunc("/attendance/scan-in", handlers.ScanIn).Methods(http.MethodPost)
	guru.HandleFunc("/attendance/scan-out", handlers.ScanOut).Methods(http.MethodPost)
	guru.HandleFunc("/attendance/history", handlers.MyAttendanceHistory).Methods(http.MethodGet)

	guru.HandleFunc("/leaves", handlers.CreateLeave).Methods(http.MethodPost)
	guru.HandleFunc("/leaves", handlers.ListMyLeaves).Methods(http.MethodGet)
	guru.HandleFunc("/teachers", handlers.ListTeachers).Methods(http.MethodGet)

	// Master data jenis cuti/izin (dipakai guru untuk mengisi dropdown form
	// pengajuan cuti) -- hanya yang aktif yang ditampilkan.
	guru.HandleFunc("/leave-types", handlers.ListActiveLeaveTypes).Methods(http.MethodGet)

	// -------------------- Admin (butuh login + role admin) --------------------
	admin := api.PathPrefix("/admin").Subrouter()
	admin.Use(middleware.JWTAuth)
	admin.Use(middleware.RequireRole("admin"))

	// Master data guru
	admin.HandleFunc("/teachers", handlers.ListTeachers).Methods(http.MethodGet)
	admin.HandleFunc("/teachers", handlers.CreateTeacher).Methods(http.MethodPost)
	admin.HandleFunc("/teachers/{id}", handlers.UpdateTeacher).Methods(http.MethodPut)
	admin.HandleFunc("/teachers/{id}", handlers.DeleteTeacher).Methods(http.MethodDelete)
	admin.HandleFunc("/teachers/{id}/activate", handlers.ActivateTeacher).Methods(http.MethodPut)

	// Master data ruangan + generator QR
	admin.HandleFunc("/rooms", handlers.ListRooms).Methods(http.MethodGet)
	admin.HandleFunc("/rooms", handlers.CreateRoom).Methods(http.MethodPost)
	admin.HandleFunc("/rooms/{id}", handlers.UpdateRoom).Methods(http.MethodPut)
	admin.HandleFunc("/rooms/{id}", handlers.DeleteRoom).Methods(http.MethodDelete)
	admin.HandleFunc("/rooms/{id}/qr", handlers.GetRoomQR).Methods(http.MethodGet)
	admin.HandleFunc("/rooms/{id}/refresh-qr", handlers.RefreshRoomQR).Methods(http.MethodPost)

	// Jadwal
	admin.HandleFunc("/schedules", handlers.ListSchedules).Methods(http.MethodGet)
	// Daftar bulan-tahun yang punya jadwal tersimpan, dipakai buat mengisi
	// filter "Bulan" di halaman Jadwal Mengajar (termasuk riwayat lama).
	admin.HandleFunc("/schedules/periods", handlers.ListSchedulePeriods).Methods(http.MethodGet)
	admin.HandleFunc("/schedules", handlers.CreateSchedule).Methods(http.MethodPost)
	admin.HandleFunc("/schedules/{id}", handlers.UpdateSchedule).Methods(http.MethodPut)
	admin.HandleFunc("/schedules/{id}", handlers.DeleteSchedule).Methods(http.MethodDelete)
	admin.HandleFunc("/schedules/{id}/duplicate", handlers.DuplicateSchedule).Methods(http.MethodPost)

	// Draft Jadwal: pengganti "Tahun Ajaran" lama -- draft berisi susunan
	// jadwal sungguhan, terpisah dari jadwal live sampai diaktifkan.
	admin.HandleFunc("/drafts", handlers.ListDrafts).Methods(http.MethodGet)
	admin.HandleFunc("/drafts", handlers.CreateDraft).Methods(http.MethodPost)
	admin.HandleFunc("/drafts/{id}", handlers.DeleteDraft).Methods(http.MethodDelete)
	admin.HandleFunc("/drafts/{id}/activate", handlers.ActivateDraft).Methods(http.MethodPut)
	admin.HandleFunc("/drafts/{id}/deactivate", handlers.DeactivateDraft).Methods(http.MethodPut)
	admin.HandleFunc("/drafts/{draftId}/schedules", handlers.ListDraftSchedules).Methods(http.MethodGet)
	admin.HandleFunc("/drafts/{draftId}/schedules/periods", handlers.ListDraftSchedulePeriods).Methods(http.MethodGet)
	admin.HandleFunc("/drafts/{draftId}/schedules", handlers.CreateDraftSchedule).Methods(http.MethodPost)
	admin.HandleFunc("/drafts/{draftId}/schedules/{id}", handlers.UpdateDraftSchedule).Methods(http.MethodPut)
	admin.HandleFunc("/drafts/{draftId}/schedules/{id}", handlers.DeleteDraftSchedule).Methods(http.MethodDelete)

	// Cuti/izin
	admin.HandleFunc("/leaves", handlers.ListAllLeaves).Methods(http.MethodGet)
	// Admin bisa langsung mengajukan+menyetujui cuti atas nama guru (tanpa
	// alur pending->approve), misalnya untuk kondisi darurat mendadak.
	admin.HandleFunc("/leaves", handlers.AdminCreateLeave).Methods(http.MethodPost)
	admin.HandleFunc("/leaves/{id}/approve", handlers.ApproveLeave).Methods(http.MethodPut)
	admin.HandleFunc("/leaves/{id}/reject", handlers.RejectLeave).Methods(http.MethodPut)

	// Master data jenis cuti/izin (dulunya hardcode di frontend, sekarang
	// dikelola Admin lewat dashboard)
	admin.HandleFunc("/leave-types", handlers.ListLeaveTypes).Methods(http.MethodGet)
	admin.HandleFunc("/leave-types", handlers.CreateLeaveType).Methods(http.MethodPost)
	admin.HandleFunc("/leave-types/{id}", handlers.UpdateLeaveType).Methods(http.MethodPut)
	admin.HandleFunc("/leave-types/{id}", handlers.DeleteLeaveType).Methods(http.MethodDelete)

	// Notifikasi: semua notifikasi yang dikirim ke pengguna + status baca-nya
	admin.HandleFunc("/notifications", handlers.AdminListNotifications).Methods(http.MethodGet)

	// Laporan
	admin.HandleFunc("/reports/monthly", handlers.MonthlyReport).Methods(http.MethodGet)
	admin.HandleFunc("/reports/history", handlers.HistoryLog).Methods(http.MethodGet)
	admin.HandleFunc("/reports/daily", handlers.DailyAttendanceReport).Methods(http.MethodGet)
	admin.HandleFunc("/reports/substitutes", handlers.SubstituteSessionsReport).Methods(http.MethodGet)
	admin.HandleFunc("/reports/annual", handlers.AnnualTeacherReport).Methods(http.MethodGet)

	// Pengaturan (panel Settings ⚙️ di dashboard admin)
	admin.HandleFunc("/settings/school", handlers.UpdateSchoolSettings).Methods(http.MethodPut)
	admin.HandleFunc("/academic-years", handlers.ListAcademicYears).Methods(http.MethodGet)
	admin.HandleFunc("/academic-years", handlers.CreateAcademicYear).Methods(http.MethodPost)
	admin.HandleFunc("/academic-years/{id}/activate", handlers.ActivateAcademicYear).Methods(http.MethodPut)
	admin.HandleFunc("/academic-years/{id}", handlers.DeleteAcademicYear).Methods(http.MethodDelete)

	return r
}
