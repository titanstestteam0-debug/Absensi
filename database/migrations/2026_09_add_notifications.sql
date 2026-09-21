-- Migrasi: tabel notifications -- notifikasi yang dikirim sistem ke pengguna
-- (guru/admin), lengkap dengan status sudah dibaca / belum dibaca.
--
-- Dibuat otomatis oleh backend saat:
--   * guru mengajukan cuti/izin        -> dikirim ke SEMUA admin aktif
--   * admin menyetujui / menolak cuti  -> dikirim ke guru yang mengajukan
--   * admin mengajukan cuti atas nama guru -> dikirim ke guru bersangkutan
--
-- Jalankan ini HANYA jika database Anda sudah pernah dibuat sebelum
-- perubahan ini ada (schema.sql yang baru sudah mengikutsertakannya untuk
-- instalasi baru, jadi skip file ini untuk instalasi baru):
--
--   mysql -u root -p absensi_guru < database/migrations/2026_09_add_notifications.sql

USE absensi_guru;

CREATE TABLE IF NOT EXISTS notifications (
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
