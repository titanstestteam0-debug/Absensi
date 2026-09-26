package models

import "time"

type Role string

const (
	RoleAdmin         Role = "admin"
	RoleGuru          Role = "guru"           // guru utama, punya jadwal tetap
	RoleGuruPengganti Role = "guru_pengganti" // guru inval, presensi menggantikan guru utama yang cuti
)

type User struct {
	ID           uint64    `json:"id"`
	Name         string    `json:"name"`
	NIP          string    `json:"nip"`
	Email        string    `json:"email,omitempty"`
	PhotoURL     string    `json:"photo_url,omitempty"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

type Room struct {
	ID              uint64     `json:"id"`
	Name            string     `json:"name"`
	QRString        string     `json:"qr_string"`
	IsActive        bool       `json:"is_active"`
	QRLastRotatedAt *time.Time `json:"qr_last_rotated_at,omitempty"`
}

// LeaveType adalah master data jenis cuti/izin yang bisa dikelola Admin lewat
// dashboard (bukan hardcode di frontend). code dipakai sebagai nilai yang
// disimpan di leaves.leave_type, label untuk ditampilkan ke pengguna.
type LeaveType struct {
	ID        uint64    `json:"id"`
	Code      string    `json:"code"`
	Label     string    `json:"label"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type Schedule struct {
	ID            uint64  `json:"id"`
	SourceDraftID *uint64 `json:"source_draft_id,omitempty"` // draft asal (nil = dibuat manual)
	TeacherID     uint64  `json:"teacher_id"`
	TeacherName   string  `json:"teacher_name,omitempty"`
	RoomID        uint64  `json:"room_id"`
	RoomName      string  `json:"room_name,omitempty"`
	DayOfWeek     int     `json:"day_of_week"`  // 1=Senin ... 7=Minggu
	PeriodMonth   int     `json:"period_month"` // 1-12, bulan berlakunya jadwal ini
	PeriodYear    int     `json:"period_year"`  // tahun berlakunya jadwal ini
	StartTime     string  `json:"start_time"`   // "HH:MM:SS"
	EndTime       string  `json:"end_time"`
	TargetJP      int     `json:"target_jp"`
	Subject       string  `json:"subject,omitempty"`
	IsActive      bool    `json:"is_active"`
}

type LeaveStatus string

const (
	LeavePending  LeaveStatus = "pending"
	LeaveApproved LeaveStatus = "approved"
	LeaveRejected LeaveStatus = "rejected"
)

type Leave struct {
	ID              uint64      `json:"id"`
	TeacherID       uint64      `json:"teacher_id"`
	StartDate       string      `json:"start_date"` // "YYYY-MM-DD"
	EndDate         string      `json:"end_date"`
	LeaveType       string      `json:"leave_type"`
	Reason          string      `json:"reason"`
	AttachmentURL   string      `json:"attachment_url,omitempty"`
	Status          LeaveStatus `json:"status"`
	RejectionReason string      `json:"rejection_reason,omitempty"`
	ApprovedBy      *uint64     `json:"approved_by,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
}

type AttendanceStatus string

const (
	AttInProgress  AttendanceStatus = "in_progress"
	AttTuntas      AttendanceStatus = "tuntas"
	AttTidakTuntas AttendanceStatus = "tidak_tuntas"
	AttTidakHadir  AttendanceStatus = "tidak_hadir"
)

type Attendance struct {
	ID                  uint64           `json:"id"`
	ScheduleID          uint64           `json:"schedule_id"`
	TeacherID           uint64           `json:"teacher_id"`
	SubstituteTeacherID *uint64          `json:"substitute_teacher_id,omitempty"`
	Date                string           `json:"date"`
	ClockIn             *time.Time       `json:"clock_in,omitempty"`
	ClockOut            *time.Time       `json:"clock_out,omitempty"`
	ActualJP            float64          `json:"actual_jp"`
	Status              AttendanceStatus `json:"status"`
	RoomIDScanned       *uint64          `json:"room_id_scanned,omitempty"`
}

// AcademicYear ("Tahun Ajaran"), mis. "2026/2027". Hanya satu yang boleh
// is_active=true dalam satu waktu -- yang lain berstatus draft (tetap bisa
// disiapkan tapi belum "berlaku").
type AcademicYear struct {
	ID        uint64 `json:"id"`
	Label     string `json:"label"`
	StartYear int    `json:"start_year"`
	EndYear   int    `json:"end_year"`
	IsActive  bool   `json:"is_active"`
}

// SchoolSettings: identitas sekolah untuk branding header aplikasi.
type SchoolSettings struct {
	SchoolName  *string `json:"school_name"`
	LogoDataURL *string `json:"logo_data_url"`
	// Subjudul kecil yang tampil di bawah nama sekolah di header aplikasi
	// (mis. "Sistem Presensi Mengajar berbasis QR Code"). NULL -> tampil
	// teks bawaan aplikasi.
	Tagline *string `json:"tagline"`
}

// Notification: pesan yang dikirim sistem ke seorang pengguna (mis. cuti
// disetujui/ditolak, pengajuan cuti baru). IsRead/ReadAt menandai apakah
// penerima sudah membukanya. UserName/UserRole hanya terisi di daftar milik
// admin (yang menampilkan semua penerima).
type Notification struct {
	ID        uint64     `json:"id"`
	UserID    uint64     `json:"user_id"`
	UserName  string     `json:"user_name,omitempty"`
	UserRole  string     `json:"user_role,omitempty"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	RefID     *uint64    `json:"ref_id,omitempty"`
	IsRead    bool       `json:"is_read"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ScheduleDraft: draft jadwal mengajar -- dinamai bebas oleh admin (mis.
// "2026/2027 V1"), diisi terpisah dari jadwal yang berlaku (schedules),
// dan baru memengaruhi jadwal LIVE saat diaktifkan (lihat draft_handler.go).
type ScheduleDraft struct {
	ID            uint64 `json:"id"`
	Name          string `json:"name"`
	IsActive      bool   `json:"is_active"`
	ScheduleCount int    `json:"schedule_count"` // jumlah baris jadwal di dalam draft ini
}

// DraftSchedule: satu baris jadwal DI DALAM sebuah draft. Strukturnya sama
// persis dengan Schedule, tapi belum "berlaku" sampai draft-nya diaktifkan.
type DraftSchedule struct {
	ID          uint64 `json:"id"`
	DraftID     uint64 `json:"draft_id"`
	TeacherID   uint64 `json:"teacher_id"`
	TeacherName string `json:"teacher_name,omitempty"`
	RoomID      uint64 `json:"room_id"`
	RoomName    string `json:"room_name,omitempty"`
	DayOfWeek   int    `json:"day_of_week"`
	PeriodMonth int    `json:"period_month"`
	PeriodYear  int    `json:"period_year"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	TargetJP    int    `json:"target_jp"`
	Subject     string `json:"subject,omitempty"`
}
