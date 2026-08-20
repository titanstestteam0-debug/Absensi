-- Migrasi: menambahkan (1) alasan penolakan pada pengajuan cuti/izin, dan
-- (2) foto profil guru yang bisa diperbarui sendiri oleh guru.
--
-- Jalankan ini HANYA jika database Anda sudah pernah dibuat sebelum kolom-kolom
-- ini ditambahkan (schema.sql yang baru sudah mengikutsertakannya untuk instalasi
-- baru, jadi skip file ini untuk instalasi baru):
--
--   mysql -u root -p absensi_guru < database/migrations/2026_08_add_leave_rejection_reason_and_photo.sql

USE absensi_guru;

ALTER TABLE leaves
    ADD COLUMN rejection_reason TEXT NULL AFTER status;

-- photo_url disimpan sebagai data URI base64 (mis. "data:image/jpeg;base64,...")
-- supaya tidak bergantung pada disk storage lokal yang tidak persisten di
-- hosting seperti Railway (filesystem-nya ephemeral, hilang tiap redeploy).
ALTER TABLE users
    ADD COLUMN photo_url LONGTEXT NULL AFTER email;
