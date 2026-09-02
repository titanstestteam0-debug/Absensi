-- Migrasi: Tahun Ajaran (bisa bikin draft tahun depan, aktifkan salah satu)
-- + Identitas Sekolah (nama & logo custom, tampil di header ganti
-- "SIM-ABSENSI GURU"). Keduanya dikelola lewat panel Pengaturan (⚙️) baru
-- di dashboard admin.
--
-- Jalankan ini HANYA jika database Anda sudah pernah dibuat sebelum
-- perubahan ini ada (schema.sql yang baru sudah mengikutsertakannya untuk
-- instalasi baru, jadi skip file ini untuk instalasi baru).
--
-- PENTING (khusus TiDB Cloud): jalankan tiap statement SATU PER SATU
-- (select baris per baris lalu Run), jangan select semuanya sekaligus --
-- lihat percakapan sebelumnya soal ini.

USE absensi_guru;

CREATE TABLE IF NOT EXISTS academic_years (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    label       VARCHAR(20)       NOT NULL UNIQUE,
    start_year  SMALLINT UNSIGNED NOT NULL,
    end_year    SMALLINT UNSIGNED NOT NULL,
    is_active   TINYINT(1)        NOT NULL DEFAULT 0,
    created_at  DATETIME          NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME          NOT NULL DEFAULT CURRENT_TIMESTAMP
                                   ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS school_settings (
    id             TINYINT UNSIGNED PRIMARY KEY,
    school_name    VARCHAR(150)  NULL,
    logo_data_url  MEDIUMTEXT    NULL,
    updated_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP
                                  ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB;

INSERT INTO school_settings (id, school_name, logo_data_url)
VALUES (1, NULL, NULL)
ON DUPLICATE KEY UPDATE id = id;
