-- Migrasi: rombak fitur "Tahun Ajaran" (yang lama, sekadar label tanpa efek
-- nyata ke jadwal) menjadi "Draft Jadwal" -- draft berisi susunan jadwal
-- sungguhan (bebas diedit terpisah dari jadwal yang berlaku), yang bisa
-- diaktifkan untuk mengisi jadwal LIVE periode terkait, dan dinonaktifkan
-- lagi untuk mengosongkannya kembali.
--
-- Jalankan ini HANYA jika database Anda sudah pernah dibuat sebelum
-- perubahan ini ada (schema.sql yang baru sudah mengikutsertakannya untuk
-- instalasi baru, jadi skip file ini untuk instalasi baru):
--
--   mysql -u root -p absensi_guru < database/migrations/2026_09_add_schedule_drafts.sql
--
-- Catatan: tabel `academic_years` lama TIDAK dihapus (data lama tetap ada,
-- kalau-kalau perlu dirujuk), tapi tidak lagi dipakai aplikasi.

USE absensi_guru;

CREATE TABLE IF NOT EXISTS schedule_drafts (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(100)      NOT NULL UNIQUE,
    is_active   TINYINT(1)        NOT NULL DEFAULT 0,
    created_at  DATETIME          NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME          NOT NULL DEFAULT CURRENT_TIMESTAMP
                                   ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS draft_schedules (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    draft_id     BIGINT UNSIGNED NOT NULL,
    teacher_id   BIGINT UNSIGNED NOT NULL,
    room_id      BIGINT UNSIGNED NOT NULL,
    day_of_week  TINYINT UNSIGNED NOT NULL,
    period_month TINYINT UNSIGNED NOT NULL,
    period_year  SMALLINT UNSIGNED NOT NULL,
    start_time   TIME            NOT NULL,
    end_time     TIME            NOT NULL,
    target_jp    INT UNSIGNED    NOT NULL,
    subject      VARCHAR(150)    NULL,
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP
                                  ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_draft_schedules_draft FOREIGN KEY (draft_id) REFERENCES schedule_drafts(id)
        ON DELETE CASCADE,
    CONSTRAINT fk_draft_schedules_teacher FOREIGN KEY (teacher_id) REFERENCES users(id)
        ON DELETE CASCADE,
    CONSTRAINT fk_draft_schedules_room FOREIGN KEY (room_id) REFERENCES rooms(id)
        ON DELETE RESTRICT,
    INDEX idx_draft_schedules_draft_period (draft_id, period_year, period_month)
) ENGINE=InnoDB;

-- Dipisah jadi dua ALTER TABLE terpisah (bukan digabung dalam satu
-- statement): TiDB kadang memvalidasi klausa ADD INDEX terhadap skema
-- SEBELUM ADD COLUMN diterapkan kalau keduanya digabung dalam satu
-- ALTER TABLE, sehingga gagal dengan "column does not exist".
ALTER TABLE schedules
    ADD COLUMN IF NOT EXISTS source_draft_id BIGINT UNSIGNED NULL AFTER id;

ALTER TABLE schedules
    ADD INDEX IF NOT EXISTS idx_schedules_source_draft (source_draft_id);
