## Tor Web Scraper (Go)

Bu proje, Tor ağı üzerinden `.onion` sitelerine erişip sayfaları bir tarayıcı gibi (JavaScript destekli) yükleyerek:
- Render edilmiş HTML içeriğini kaydeder
- Tam sayfa ekran görüntüsü (screenshot) alır

Çıktılar `results/` klasörüne, işlem kayıtları ise `scan_report.log` dosyasına yazılır.

## Özellikler

- **Otomatik Tor port tespiti**: `9050` / `9150` portlarını dener ve aktif olanı kullanır.
- **Tor proxy desteği (SOCKS5)**: HTTP kontrolü ve ChromeDP navigasyonları Tor üzerinden yapılır.
- **Headless tarayıcı**: `chromedp` ile sayfalar gerçek bir tarayıcı gibi işlenir (JS dahil).
- **Otomatik çıktı yönetimi**: Zaman damgalı HTML (`.html`) ve screenshot (`.png`) üretilir.
- **Loglama**: Başarılı/başarısız taramalar `scan_report.log` dosyasına eklenir.
- **YAML yapılandırma**: Hedefler `sites.yaml` dosyasından okunur.

## Gereksinimler

- **Go**: Sisteminizde Go kurulu olmalı.
- **Tor**: Tor Browser açık olmalı (veya bir Tor servisi çalışıyor olmalı).
- **Chrome/Chromium**: `chromedp` için sistemde Chrome/Chromium bulunmalı.

## Kurulum

Proje klasöründe:

```bash
go mod tidy
```

## Yapılandırma (`sites.yaml`)

`sites.yaml` içinde taranacak siteleri aşağıdaki formatta tanımlayın:

```yaml
sites:
  - isim: "Example"
    url: "http://exampleonionsite.onion/"
```

## Kullanım

Programı çalıştırmadan önce **Tor Browser'ın açık** olduğundan emin olun.

```bash
go run main.go
```

Uygulama, `sites.yaml` içindeki hedefleri bir menü olarak gösterir:
- **1..N** arası seçim ile bir hedefi çalıştırır
- **0** veya `q` / `quit` / `exit` ile çıkar
- Hata durumunda mesajı gösterip tekrar menüye döner

## Çıktılar

- **`results/`**: Her tarama için zaman damgalı dosyalar
  - `results/<isim>_<timestamp>.html`
  - `results/<isim>_<timestamp>.png`
- **`scan_report.log`**: Tarama geçmişi (RFC3339 zaman damgası ile)

## Notlar

- **Timeout**: Onion siteleri yavaş olabilir; ChromeDP tarafında varsayılan timeout **120 saniye** olarak ayarlanmıştır.
- **Yasal sorumluluk**: Bu araç eğitim/araştırma amaçlıdır. Kullanım sorumluluğu kullanıcıya aittir.
