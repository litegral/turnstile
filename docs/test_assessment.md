# Backend Engineering Technical Assessment

## Overview

Anda akan berperan sebagai engineer yang bertanggung jawab membangun dan mengelola sistem Ticket Booking untuk sebuah konser berskala besar.

Sistem ini memungkinkan ribuan pengguna melakukan pembelian tiket secara bersamaan melalui website maupun aplikasi mobile. Selama periode penjualan, sistem harus mampu menangani lonjakan traffic yang tinggi, menjaga konsistensi data, mencegah overselling, serta memastikan setiap transaksi diproses dengan benar meskipun terjadi kegagalan jaringan atau gangguan pada layanan eksternal.

Melalui assessment ini, kami ingin mengevaluasi kemampuan Anda dalam menganalisis permasalahan yang umum terjadi pada sistem berskala besar, serta kemampuan Anda dalam merancang solusi yang reliable, scalable, dan maintainable.

### Expected Deliverables

Untuk setiap studi kasus, mohon:

1. Jelaskan analisis Anda terhadap permasalahan yang diberikan.
2. Jelaskan asumsi yang digunakan.
3. Berikan solusi yang Anda rekomendasikan beserta trade-off yang mungkin muncul.
4. Jika diperlukan, sertakan diagram, pseudocode, atau contoh implementasi untuk memperjelas jawaban.

### Time Allocation

Estimasi waktu pengerjaan: **3–4 hari**

Kami lebih menghargai kualitas analisis, kejelasan penjelasan, dan kemampuan berpikir sistematis dibandingkan panjang jawaban atau kompleksitas solusi yang diberikan.

---

## Section 1 – Race Condition

### Scenario

Sebuah platform penjualan tiket konser memiliki 1 tiket VIP yang tersisa.

Dua pengguna atau lebih melakukan pembelian pada waktu yang hampir bersamaan.

Proses saat ini:

1. Sistem membaca jumlah tiket yang tersedia.
2. Jika tiket masih tersedia, sistem mengurangi stok sebanyak 1.
3. Sistem menyimpan transaksi pembelian.

Setelah kedua transaksi selesai diproses, ternyata kedua pengguna berhasil mendapatkan tiket.

Bagaimana cara untuk membuat hanya 1 transaction yang berhasil dan yang satunya gagal

---

## Section 2 – High Traffic Processing

### Scenario

Sebuah aplikasi menerima lebih dari 10.000 transaksi dalam waktu kurang dari 1 menit.

Bagaimana cara setiap transaction success tersimpan di database tanpa ada satupun yang gagal.

---

## Section 3 – External API Integration

### Scenario

Setiap transaksi yang berhasil harus dikirim ke layanan pihak ketiga dalam hal ini accounting software.

Sistem mengirim request berikut:

```http
POST /transaction
```

Namun layanan pihak ketiga mengembalikan bukan success response seperti contoh.

```http
HTTP 500 Internal Server Error.
```

Apa yang harus dilakukan untuk memastikan semua transaction success terkirim dan diterima oleh pihak ketiga.

---

## Section 4 – Duplicate Request

### Scenario

Saat mengirimkan transaction ke pihak ketiga, pihak ketiga akan process di background dan akan mengirimkan hook berupa data payment yang harus di simpan ke trasaction_payment.

Karena masalah jaringan di internal system, pihak ketiga tidak menerima respon sesuai harapan mereka dan mereka menggunakan sistem retry untuk mengirim data kembali

Karena anomali di sistem pihak ketiga, mereka kemudian mengirim ulang 2 request di waktu yang sama dan memiliki data yang sama.

Bagaimana menghandle process ini sehingga data tidak duplikat.

---

## Section 5 – Data Synchronization

### Scenario

Tiket sistem harus mengirim update ketersediaan tiket data ke sistem lain.

Update pertama:

```text
Quantity = 5
```

Update kedua:

```text
Quantity = 2
```

Karena latency jaringan, update kedua diterima terlebih dahulu, kemudian update pertama.

Akibatnya data pada sistem tujuan menjadi tidak sesuai dengan kondisi sebenarnya yang seharusnya system tujuan memunculkan quantity 2 tetapi memunculkan 5.

Bagaimana cara mencegah ini terjadi?

---

## Tugas

1. Buatkan Diagram flow untuk setiap scenario dan di gabung kan menjadi satu kesatuan
2. Implementasikan service sederhana setiap Scenario dalam satu aplikasi utuh

### Catatan

Bahasa pemrograman bebas (Go di sarankan).

Mohon sertakan:

- Source code (public github)
- Cara menjalankan aplikasi
- Cara melakukan testing
- Penjelasan singkat mengenai desain yang digunakan
