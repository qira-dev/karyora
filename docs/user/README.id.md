# Karyora — Panduan Pengguna

Panduan ini menjelaskan aplikasi desktop Karyora sebagaimana dikirimkan saat ini. Panduan ini hanya menyatakan apa yang benar-benar dilakukan produk; kemampuan yang belum ada didaftarkan sebagai keterbatasan dan tidak pernah disajikan sebagai janji. Panduan yang sama tersedia dalam 12 bahasa di folder ini.

## 1. Apa itu Karyora

- Karyora adalah aplikasi desktop yang mengutamakan penyimpanan lokal (local-first). Paket pengetahuan, pekerjaan (job), dan pengaturan Anda tersimpan di komputer Anda sendiri.
- Produk ini bekerja tanpa layanan cloud apa pun dari proyek. Satu-satunya lalu lintas jaringan yang dapat dihasilkannya adalah lalu lintas yang Anda mulai: permintaan ke penyedia AI Anda, impor paket pengetahuan yang Anda minta, dan — bila Anda mengonfigurasinya — kanal pembaruan.
- Fitur AI menggunakan akun penyedia milik Anda sendiri (BYOK, bring your own key). Anda mengonfigurasi kunci API Anda sendiri, dan penyedia menagih Anda secara langsung. Karyora tidak pernah menjual ulang penggunaan penyedia dan tidak pernah menjadi perantara pembayarannya.

## 2. Build komunitas dan build resmi

- Kode sumber Karyora dirilis di bawah Apache License 2.0. Anda boleh menggunakannya, mempelajarinya, mengubahnya, dan mendistribusikannya ulang sesuai lisensi tersebut, termasuk membangunnya sendiri.
- Repositori publiknya adalah `https://github.com/qira-dev/karyora`. Setiap rilis ditandai dengan tag `v*`, dan pohon sumber pada tag tersebut adalah sumber rilis itu.
- Rilis "resmi" adalah rilis yang dibangun oleh pipeline build proyek sendiri pada revisi bertag, dengan bukti yang terekam. Build resmi dan build komunitas memiliki fitur yang sama; perbedaannya adalah siapa yang membangun artefak dan siapa yang bertanggung jawab atas artefak itu, bukan apa yang bisa dilakukan artefak tersebut.
- Pada v1 belum ada halaman unduhan resmi maupun kanal pembaruan resmi. Sebelum keduanya dipublikasikan, Anda dapat memperoleh Karyora dari repositori publik dan membangunnya dari kode sumber.

## 3. Platform yang didukung (v1)

| OS | CPU | Batas bawah |
|---|---|---|
| Windows 11 | x86_64 | Windows 11, 23H2 atau lebih baru |
| Windows 11 | ARM64 | Windows 11 untuk ARM |
| macOS | x86_64 | macOS 15 (Sequoia) atau lebih baru |
| macOS | ARM64 | macOS 15 (Sequoia) atau lebih baru, Apple Silicon |
| Linux | x86_64 | glibc 2.34 atau lebih baru; Ubuntu LTS 22.04/24.04, Debian 12+, RHEL 9+ |
| Linux | ARM64 | glibc 2.34 atau lebih baru; keluarga distribusi yang sama |

Tidak didukung secara eksplisit pada v1: Windows 10; macOS 14 atau lebih lama; distribusi berbasis musl seperti Alpine; setiap kombinasi yang tidak tercantum di atas. Dukungan mengikuti jendela dukungan keamanan masing-masing vendor dan diperiksa ulang pada setiap rilis.

## 4. Paket dan cara memverifikasinya

- Set paket yang dihasilkan adalah: MSI Windows, pkg macOS, deb Linux, rpm Linux, dan arsip portabel Linux — satu artefak untuk setiap arsitektur CPU yang didukung.
- Setiap paket dibuat bersama manifes `SHA256SUMS` serta catatan SBOM dan provenans per artefak. Cara memverifikasi paket: hitung digest sha256 dari unduhan Anda dan bandingkan dengan entri di manifes. Jika digest tidak cocok, jangan instal paket itu.
- Paket tidak ditandatangani secara digital pada v1 — memang dirancang demikian. Langkah verifikasi tanda tangan tingkat platform (pemeriksaan tanda tangan sistem operasi, kunci repositori) akan didokumentasikan bersama rilis bertanda tangan pertama. Sebelum itu, perbandingan digest di atas adalah verifikasi yang dapat Anda lakukan.
- Berkas `LICENSE` dan `NOTICE`, yang memuat lisensi proyek dan atribusi pihak ketiga, adalah bagian dari repositori sumber.

## 5. Pembaruan

- Karyora memiliki mekanisme pembaruan, dan pada v1 mekanisme itu dikirimkan dalam keadaan nonaktif secara bawaan. Tidak ada kanal pembaruan yang dikonfigurasi, sehingga pemeriksaan pembaruan melaporkan penolakan bertipe yang menyatakan bahwa tidak ada endpoint yang dikonfigurasi. Ini disengaja: kanal pembaruan resmi hanya akan diaktifkan setelah dipublikasikan.
- Sebelum itu, memperbarui berarti menginstal paket yang lebih baru di atas paket lama. Data Anda terjaga lintas pembaruan (lihat bagian 6).

## 6. Data Anda

- Semua data produk berada di mesin Anda. Direktori data bawaan adalah `%APPDATA%\Karyora\data` di Windows, `~/Library/Application Support/Karyora/data` di macOS, dan `~/.local/share/karyora/data` di Linux (pengaturan XDG `XDG_DATA_HOME` dihormati). Anda dapat memilih lokasi lain dengan flag `--data-dir`.
- Produk tidak melakukan pencadangan terjadwal. Untuk mencadangkan: hentikan produk, lalu salin direktori data. Memulihkan berarti mengembalikan direktori salinan ke tempatnya. Tidak ada pencadangan cloud.
- Mencopot pemasangan atau memperbarui tidak menghapus data Anda. Menghapus data adalah tindakan terpisah dan eksplisit yang Anda lakukan sendiri.
- Data saat tersimpan (data at rest) tidak dienkripsi. Lindungi direktori data dengan sarana Anda sendiri, misalnya enkripsi disk penuh. Ini adalah keterbatasan yang diketahui, dinyatakan terus terang.
- Kredensial penyedia (kunci API Anda) hanya disimpan di keyring sistem operasi Anda. Kredensial itu tidak pernah ditulis ke berkas, log, atau isi prompt.

## 7. Privasi

- Produk ini tidak memuat telemetri: tidak ada yang dikirim ke server proyek.
- Diagnostik dihasilkan secara lokal dan secara bawaan menjaga privasi; Anda yang menentukan apakah ada sesuatu yang keluar dari mesin Anda.

## 8. Keterbatasan yang diketahui pada v1

- Eksekusi plugin pihak ketiga dinonaktifkan.
- Operasi tim dan jarak jauh tidak tersedia.
- Memindahkan pekerjaan aktif dari satu mesin ke mesin lain tidak didukung.
- Sinkronisasi paket pengetahuan portabel tidak tersedia.
- Fitur pengelompokan "Work" tidak ada.
- Tidak ada langganan berbayar dan tidak ada hak (entitlement): tidak ada yang bisa dibeli, tidak ada tingkatan berbayar.
- Tidak ada janji angka kinerja apa pun.
- Data saat tersimpan tidak dienkripsi (lihat bagian 6).
- Belum ada halaman unduhan resmi maupun kanal pembaruan (lihat bagian 2 dan 5).

## 9. Lisensi

- Karyora dirilis di bawah Apache License 2.0. Teks lisensi lengkap ada di berkas `LICENSE`, dan atribusi pihak ketiga ada di berkas `NOTICE`; keduanya berada di repositori sumber.
- Membangun Karyora sendiri, memfork-nya, atau mendistribusikannya ulang sesuai lisensi mempertahankan semua hak yang diberikan lisensi. Hak ini tidak bergantung pada pembelian apa pun, karena memang tidak ada yang bisa dibeli.

## 10. Referensi baris perintah

Alat baris perintahnya adalah `uawp`.

| Perintah | Fungsinya |
|---|---|
| `uawp start` | Memulai layanan lokal. Start kedua pada direktori data yang sama ditolak selama masih ada yang berjalan. |
| `uawp stop` | Menghentikan layanan dengan pematian terbatas yang tertib. |
| `uawp status` | Melaporkan apakah layanan sedang berjalan. |
| `uawp doctor` | Menjalankan pemeriksaan kesehatan dan diagnostik lokal. |
| `uawp update check` / `uawp update apply` / `uawp update status` | Memeriksa, menerapkan, atau melaporkan pembaruan. Pada v1, tanpa kanal yang dikonfigurasi, check dan apply berakhir dengan penolakan bertipe yang menyatakan tidak ada endpoint yang dikonfigurasi; `status` melaporkan status pembaruan. |
| `uawp job submit` / `uawp job status` / `uawp job events` | Mengirimkan job yang tahan lama, melaporkan statusnya, atau mengikuti peristiwanya. |
| `uawp credential store --provider <id> [--name api-key]` | Menyimpan kunci API penyedia Anda di keyring OS. |
| `uawp credential status` | Melaporkan apakah kredensial ada — tidak pernah nilainya. |
| `uawp credential remove` | Menghapus kredensial yang tersimpan. |
| `uawp version` / `uawp help` | Mencetak versi atau bantuan. |

Kredensial yang tersimpan saja tidak membuka apa pun: panggilan nyata ke penyedia tetap harus melewati aturan penanganan data yang diberlakukan produk untuk penyedia tersebut.
