-- =====================================================================
-- Sistem Absensi Jam Mengajar Guru berbasis QR Code
-- Skema Database MySQL
-- =====================================================================

CREATE DATABASE IF NOT EXISTS absensi_guru
  CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE absensi_guru;

-- ---------------------------------------------------------------------
-- users: Admin, Guru, dan Guru Pengganti (role membedakan hak akses)
-- ---------------------------------------------------------------------
CREATE TABLE users (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name            VARCHAR(150)        NOT NULL,
    nip             VARCHAR(50)         NOT NULL UNIQUE,
    email           VARCHAR(150)        NULL UNIQUE,
    photo_url       LONGTEXT            NULL,        -- foto profil (data URI base64), diisi guru sendiri
    password_hash   VARCHAR(255)        NOT NULL,
    role            ENUM('admin', 'guru', 'guru_pengganti') NOT NULL DEFAULT 'guru',
    is_active       TINYINT(1)          NOT NULL DEFAULT 1,
    created_at      DATETIME            NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME            NOT NULL DEFAULT CURRENT_TIMESTAMP
                                         ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_users_role (role)
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- rooms: Ruangan kelas, masing-masing punya QR string unik
-- ---------------------------------------------------------------------
-- qr_string TIDAK pernah berubah otomatis -- hanya berganti kalau admin
-- menekan tombol "Refresh Sekarang" di halaman QR Live (lihat
-- handlers/room_handler.go), supaya QR yang sudah dicetak/ditempel di kelas
-- tetap valid dan bisa dipakai berulang kali.
CREATE TABLE rooms (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name                VARCHAR(100)    NOT NULL,
    qr_string           VARCHAR(191)    NOT NULL UNIQUE,
    qr_last_rotated_at  DATETIME        NULL,         -- kapan terakhir kali qr_string diganti (dibuat/direfresh manual)
    is_active           TINYINT(1)      NOT NULL DEFAULT 1,
    created_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP
                                         ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- settings: Konfigurasi sistem (key-value)
-- Contoh: jp_duration_minutes = 45, early_scan_tolerance_minutes = 60
-- ---------------------------------------------------------------------
CREATE TABLE settings (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    config_key   VARCHAR(100)  NOT NULL UNIQUE,
    config_value VARCHAR(255)  NOT NULL,
    description  VARCHAR(255)  NULL,
    updated_at   DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP
                                ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB;

INSERT INTO settings (config_key, config_value, description) VALUES
    ('jp_duration_minutes', '45', 'Durasi aktual 1 Jam Pelajaran (menit)'),
    ('early_scan_tolerance_minutes', '60', 'Batas toleransi scan masuk sebelum jadwal dimulai (menit)');

-- ---------------------------------------------------------------------
-- leave_types: Master data jenis cuti/izin, dikelola Admin lewat dashboard
-- (bukan hardcode di kode frontend). code dipakai sebagai nilai yang
-- disimpan di leaves.leave_type, label untuk ditampilkan ke pengguna.
-- ---------------------------------------------------------------------
CREATE TABLE leave_types (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code         VARCHAR(50)   NOT NULL UNIQUE,
    label        VARCHAR(100)  NOT NULL,
    is_active    TINYINT(1)    NOT NULL DEFAULT 1,
    created_at   DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP
                                ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB;

INSERT INTO leave_types (code, label) VALUES
    ('sakit', 'Sakit'),
    ('dinas_luar', 'Izin Dinas Luar');

-- ---------------------------------------------------------------------
-- schedules: Jadwal mengajar per guru
-- day_of_week: 1=Senin ... 7=Minggu
-- ---------------------------------------------------------------------
CREATE TABLE schedules (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    teacher_id   BIGINT UNSIGNED NOT NULL,
    room_id      BIGINT UNSIGNED NOT NULL,
    day_of_week  TINYINT UNSIGNED NOT NULL,
    period_month TINYINT UNSIGNED NOT NULL,       -- 1-12, bulan berlakunya jadwal ini
    period_year  SMALLINT UNSIGNED NOT NULL,      -- tahun berlakunya jadwal ini
    start_time   TIME            NOT NULL,
    end_time     TIME            NOT NULL,
    target_jp    INT UNSIGNED    NOT NULL,
    subject      VARCHAR(150)    NULL,
    is_active    TINYINT(1)      NOT NULL DEFAULT 1,
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP
                                  ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_schedules_teacher FOREIGN KEY (teacher_id) REFERENCES users(id)
        ON DELETE CASCADE,
    CONSTRAINT fk_schedules_room FOREIGN KEY (room_id) REFERENCES rooms(id)
        ON DELETE RESTRICT,
    INDEX idx_schedules_teacher_day (teacher_id, day_of_week),
    INDEX idx_schedules_room_day (room_id, day_of_week),
    INDEX idx_schedules_period (period_year, period_month)
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- leaves: Pengajuan cuti/izin guru
-- ---------------------------------------------------------------------
CREATE TABLE leaves (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    teacher_id    BIGINT UNSIGNED NOT NULL,
    start_date    DATE            NOT NULL,
    end_date      DATE            NOT NULL,
    leave_type    VARCHAR(50)     NOT NULL,
    reason        TEXT            NULL,
    attachment_url VARCHAR(255)   NULL,
    status        ENUM('pending', 'approved', 'rejected') NOT NULL DEFAULT 'pending',
    rejection_reason TEXT         NULL,               -- diisi admin saat menolak (wajib dari sisi aplikasi)
    approved_by   BIGINT UNSIGNED NULL,
    approved_at   DATETIME        NULL,
    created_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP
                                   ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_leaves_teacher FOREIGN KEY (teacher_id) REFERENCES users(id)
        ON DELETE CASCADE,
    CONSTRAINT fk_leaves_approver FOREIGN KEY (approved_by) REFERENCES users(id)
        ON DELETE SET NULL,
    INDEX idx_leaves_teacher_status (teacher_id, status)
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- attendances: Log absensi (scan in / scan out) per jadwal per tanggal
-- ---------------------------------------------------------------------
CREATE TABLE attendances (
    id                    BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    schedule_id           BIGINT UNSIGNED NOT NULL,
    teacher_id            BIGINT UNSIGNED NOT NULL,       -- guru pemilik jadwal (untuk keperluan rekap per guru)
    substitute_teacher_id BIGINT UNSIGNED NULL,           -- diisi dengan ID guru pengganti jika ia yang scan, bukan guru utama
    date                  DATE            NOT NULL,
    clock_in              DATETIME        NULL,
    clock_out             DATETIME        NULL,
    actual_jp             DECIMAL(5,2)    NULL DEFAULT 0,
    status                ENUM('in_progress', 'tuntas', 'tidak_tuntas', 'tidak_hadir')
                                          NOT NULL DEFAULT 'in_progress',
    room_id_scanned       BIGINT UNSIGNED NULL,           -- ruangan aktual tempat scan terjadi
    created_at            DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP
                                           ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_att_schedule FOREIGN KEY (schedule_id) REFERENCES schedules(id)
        ON DELETE CASCADE,
    CONSTRAINT fk_att_teacher FOREIGN KEY (teacher_id) REFERENCES users(id)
        ON DELETE CASCADE,
    CONSTRAINT fk_att_substitute FOREIGN KEY (substitute_teacher_id) REFERENCES users(id)
        ON DELETE SET NULL,
    CONSTRAINT fk_att_room FOREIGN KEY (room_id_scanned) REFERENCES rooms(id)
        ON DELETE SET NULL,
    UNIQUE KEY uq_attendance_per_day (schedule_id, date),
    INDEX idx_att_teacher_date (teacher_id, date)
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- academic_years: Tahun Ajaran (mis. "2026/2027"). Hanya SATU yang boleh
-- aktif dalam satu waktu -- tahun ajaran lain tetap bisa dibuat/disiapkan
-- sebagai draft (mis. tahun depan), tapi belum "berlaku" sampai admin
-- mengaktifkannya lewat panel Pengaturan.
-- ---------------------------------------------------------------------
CREATE TABLE academic_years (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    label       VARCHAR(20)       NOT NULL UNIQUE,  -- "2026/2027"
    start_year  SMALLINT UNSIGNED NOT NULL,
    end_year    SMALLINT UNSIGNED NOT NULL,
    is_active   TINYINT(1)        NOT NULL DEFAULT 0,
    created_at  DATETIME          NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME          NOT NULL DEFAULT CURRENT_TIMESTAMP
                                   ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB;

-- ---------------------------------------------------------------------
-- school_settings: identitas sekolah (nama & logo) untuk branding header
-- aplikasi. Selalu SATU baris (id=1). Logo disimpan sebagai data URL
-- base64 langsung di database (tidak perlu storage/file server terpisah).
-- ---------------------------------------------------------------------
CREATE TABLE school_settings (
    id             TINYINT UNSIGNED PRIMARY KEY,
    school_name    VARCHAR(150)  NULL,
    logo_data_url  MEDIUMTEXT    NULL,
    updated_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP
                                  ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB;

INSERT INTO school_settings (id, school_name, logo_data_url) VALUES (1, NULL, NULL);

-- ---------------------------------------------------------------------
-- notifications: notifikasi yang dikirim sistem ke pengguna (guru/admin),
-- mis. pengajuan cuti baru (ke admin) atau cuti disetujui/ditolak (ke guru).
-- is_read/read_at menandai apakah penerima sudah membukanya atau belum.
-- ---------------------------------------------------------------------
CREATE TABLE notifications (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id     BIGINT UNSIGNED NOT NULL,          -- penerima notifikasi
    type        VARCHAR(50)     NOT NULL,          -- leave_submitted | leave_approved | leave_rejected | leave_admin_created | ...
    title       VARCHAR(150)    NOT NULL,
    message     TEXT            NOT NULL,
    ref_id      BIGINT UNSIGNED NULL,              -- id entitas terkait (mis. leaves.id)
    is_read     TINYINT(1)      NOT NULL DEFAULT 0,
    read_at     DATETIME        NULL,              -- kapan penerima membukanya
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_notif_user FOREIGN KEY (user_id) REFERENCES users(id)
        ON DELETE CASCADE,
    INDEX idx_notif_user_read (user_id, is_read, created_at),
    INDEX idx_notif_created (created_at)
) ENGINE=InnoDB;
