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
CREATE TABLE rooms (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name                VARCHAR(100)    NOT NULL,
    qr_string           VARCHAR(191)    NOT NULL UNIQUE,
    qr_expires_at       DATETIME        NULL,        -- kapan qr_string saat ini kedaluwarsa & wajib rotasi
    qr_last_rotated_at  DATETIME        NULL,         -- kapan terakhir kali qr_string diganti
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
    ('early_scan_tolerance_minutes', '60', 'Batas toleransi scan masuk sebelum jadwal dimulai (menit)'),
    ('qr_rotation_seconds', '20', 'Interval otomatis QR Code ruangan berganti (detik), agar QR yang difoto/discreenshot cepat kedaluwarsa dan tidak bisa dipakai scan dari rumah');

-- ---------------------------------------------------------------------
-- schedules: Jadwal mengajar per guru
-- day_of_week: 1=Senin ... 7=Minggu
-- ---------------------------------------------------------------------
CREATE TABLE schedules (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    teacher_id   BIGINT UNSIGNED NOT NULL,
    room_id      BIGINT UNSIGNED NOT NULL,
    day_of_week  TINYINT UNSIGNED NOT NULL,
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
    INDEX idx_schedules_room_day (room_id, day_of_week)
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
