package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"


	"github.com/chromedp/chromedp" // Sayfayı render eder ve çıktı toplar.
	"golang.org/x/net/proxy" // Tor yönlendirmesi için SOCKS5 dialer üretir.
	"gopkg.in/yaml.v3" // sites.yaml konfigürasyonunu sağlar.
)

// site, `sites.yaml` içinde tanımlanan hedefleri temsil eder.
type site struct {
	Isim string `yaml:"isim"`
	Url  string `yaml:"url"`
}

// YAML konfigürasyon dosya yapısı.
type Config struct {
	Site []site `yaml:"sites"`
}

// Yaygın Tor SOCKS portlarını (9050 ve 9150) test eder.
func findActiveTorPort() (string, error) {
	ports := []string{"9050", "9150"}

	for _, port := range ports {
		fmt.Printf("[TEST] Port %s kontrolleri gerçekleştiriliyor...\n", port)
		timeout := 2 * time.Second
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, timeout)
		if err == nil {
			conn.Close()
			fmt.Printf("[BAŞARILI✅] Port %s AÇIK!\n", port)
			return port, nil
		}
		fmt.Printf("[BAŞARISIZ❌] Port %s KAPALI!\n", port)
	}
	return "", fmt.Errorf("Tor portları (9050, 9150) açık değil. Tor Browser'ın açık olduğundan emin olduktan sonra tekrar deneyin.")
}


func createTorClient(torPort string) (*http.Client, error) {
	// Aktif Tor portunu hedefleyen bir SOCKS5 dialer oluştur.
	torAddr := "127.0.0.1:" + torPort
	dialer, err := proxy.SOCKS5("tcp", torAddr, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("SOCKS5 proxy hatası.❌: %v", err)
	}

	// HTTP transport, TCP bağlantılarını Tor üzerinden kurmak için SOCKS5 dialer kullanır.
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	}

	client := &http.Client{
		Transport: transport,
		// Tor browser yavaş çalıştığından verinin daha sağlıklısı alınabilmesi için eklenen timeout.
		Timeout: 40 * time.Second,
	}

	return client, nil
}

// ChromeDP ile bir Chromium örneği başlatır.
// Not: Onion sayfalar yavaş olduğu için 90 saniye timeout uygulanır.
func captureWithTor(siteUrl, siteName string, torPort string) ([]byte, string, error) {
	// Tüm navigasyon isteklerinin Tor üzerinden geçebilmesi için ChromeDP allocator seçenekleri Proxy tarayıcı seviyesinde set edilir.
	proxyURL := fmt.Sprintf("socks5://127.0.0.1:%s", torPort)
	fmt.Printf("[ChromeDP] Proxy kullanılıyor: %s\n", proxyURL)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ProxyServer(proxyURL),
		
		// Bu flag'ler gereksiz engellemeleri azaltmaya yardımcı olur.
		chromedp.Flag("disable-web-security", true),
		chromedp.Flag("disable-features", "IsolateOrigins,site-per-process"),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("ignore-certificate-errors", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Onion siteler yavaş olduğundan sayfanın tamamen yüklenmesi için timeout.
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	var screenshot []byte
	var htmlContent string

	// Kısa bekleme 15s ardından HTML + screenshot yakalama.
	err := chromedp.Run(ctx,
		chromedp.Navigate(siteUrl),
		chromedp.Sleep(15*time.Second),
		chromedp.OuterHTML("html", &htmlContent),
		chromedp.FullScreenshot(&screenshot, 100),
	)

	if err != nil {
		return nil, "", err
	}

	return screenshot, htmlContent, nil
}

// saveHTML, yakalanan HTML'i zaman belirterek yeni bir dosya adıyla `results/` klasörüne kaydeder.
func saveHTML(siteName, url, content string) error {
	// Çıktı klasörü yoksa oluşturur.
	os.MkdirAll("results", 0755)

	timestamp := time.Now().Format("20060102_150405")
	// Dosya adını OS uyumlu hale getirir .
	safeName := strings.ReplaceAll(siteName, " ", "_")
	filename := fmt.Sprintf("results/%s_%s.html", safeName, timestamp)

	err := os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		return err
	}

	fmt.Printf("[KAYIT] HTML: %s\n", filename)
	return nil
}

// saveScreenshot, yakalanan screenshot'u zaman belirterek yeni bir dosya adıyla `results/` klasörüne kaydeder.
func saveScreenshot(siteName string, screenshot []byte) error {
	// Çıktı klasörü yoksa oluşturur.
	os.MkdirAll("results", 0755)

	timestamp := time.Now().Format("20060102_150405")
	// Dosya adını OS uyumlu hale getirir.
	safeName := strings.ReplaceAll(siteName, " ", "_")
	filename := fmt.Sprintf("results/%s_%s.png", safeName, timestamp)

	err := os.WriteFile(filename, screenshot, 0644)
	if err != nil {
		return err
	}

	fmt.Printf("[KAYIT] Screenshot: %s\n", filename)
	return nil
}

// writeLog, `scan_report.log` dosyasına tek bir satır ekler.
func writeLog(message string) {
	f, err := os.OpenFile("scan_report.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("[UYARI❗] Log yazılamadı.: %v\n", err)
		return
	}
	defer f.Close()
	logEntry := fmt.Sprintf("[%s] %s\n", time.Now().Format(time.RFC3339), message)
	f.WriteString(logEntry)
}

// loadConfig, verilen path'ten YAML konfigürasyonunu okur.
func loadConfig(path string) (Config, error) {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("Dosya okunurken hata oluştu.❌: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(yamlFile, &config); err != nil {
		return Config{}, fmt.Errorf("YAML işleme hatası.❌: %w", err)
	}

	return config, nil
}

// printMenuAndSelectSite, basit bir interaktif menü yazdırır.
func printMenuAndSelectSite(reader *bufio.Reader, sites []site, lastErr error) (selectedIndex int, exit bool, err error) {
	fmt.Println("------------------------------------------------")
	fmt.Println("Kayıtlı sitelerden seçim yapın:")
	if lastErr != nil {
		fmt.Printf("[HATA❌] Önceki deneme başarısız oldu: %v\n", lastErr)
	}
	if len(sites) == 0 {
		return 0, false, fmt.Errorf("sites.yaml içeriği boş..🥺")
	}

	for i, s := range sites {
		fmt.Printf("  %d) %s (%s)\n", i+1, s.Isim, s.Url)
	}
	fmt.Println("  0) Çıkış")
	fmt.Print("Seçiminiz: ")

	line, readErr := reader.ReadString('\n')
	if readErr != nil {
		// stdin kapalıysa sonsuz döngüye girme.
		if readErr == io.EOF {
			line = strings.TrimSpace(line)
			if line == "" {
				return 0, true, nil
			}
			
		} else {
			return 0, false, fmt.Errorf("Girdi okunamadı❌: %w", readErr)
		}
	}
	line = strings.TrimSpace(line)

	if line == "" {
		return 0, false, fmt.Errorf("Boş seçim girdiniz..🥺")
	}
	switch strings.ToLower(line) {
	case "q", "quit", "exit", "cikis", "çıkış":
		return 0, true, nil
	}

	n, convErr := strconv.Atoi(line)
	if convErr != nil {
		return 0, false, fmt.Errorf("Geçersiz seçim🥺: %q (0-%d arası sayı girin)", line, len(sites))
	}
	if n == 0 {
		return 0, true, nil
	}
	if n < 1 || n > len(sites) {
		return 0, false, fmt.Errorf("Geçersiz seçim🥺: %d (1-%d arası seçin)", n, len(sites))
	}
	return n - 1, false, nil
}

// scrapeSingleSite, verilen hedef için tek bir tarama yapar.
// - Tor'a yönlenmiş HTTP istemcisi ile hızlı HTTP status kontrolü yapar.
// - Çıktıları `results/` altına kaydeder.
// - Hata döndürerek çağıranın log yazıp menüyü yeniden göstermesini sağlar.
func scrapeSingleSite(torClient *http.Client, torPort string, s site) error {
	fmt.Println("------------------------------------------------")
	fmt.Printf("Hedef: %s | URL: %s\n", s.Isim, s.Url)

	// 1) ChromeDP'yi başlatmadan önce hızlı status kontrolü yapar.
	resp, err := torClient.Get(s.Url)
	if err != nil {
		return fmt.Errorf("Site erişilemez🥺: %w", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	fmt.Printf("[DURUM] HTTP Status: %d %s\n", resp.StatusCode, resp.Status)
	if resp.StatusCode != 200 {
		fmt.Println("[UYARI❗] Sayfa 200 döndürmedi, yine de ChromeDP deneniyor...🥺")
	}

	// 2) ChromeDP'yi başlat ve sayfayı başlatır.
	fmt.Println("[İŞLEM] ChromeDP başlatılıyor...")
	screenshot, htmlContent, err := captureWithTor(s.Url, s.Isim, torPort)
	if err != nil {
		return fmt.Errorf("ChromeDP hatası❌: %w", err)
	}

	// 3) Çıktıları kalıcı hale getirir.
	if err := saveHTML(s.Isim, s.Url, htmlContent); err != nil {
		return fmt.Errorf("HTML kaydedilemedi🥺: %w", err)
	}
	if err := saveScreenshot(s.Isim, screenshot); err != nil {
		return fmt.Errorf("screenshot kaydedilemedi🥺: %w", err)
	}

	return nil
}

func main() {
	// 1) Aktif Tor SOCKS portunu tespit et (9150, 9050).
	activePort, err := findActiveTorPort()
	if err != nil {
		log.Fatal("[❌KRİTİK HATA❌] Tor bağlantısı sağlanamadı: ", err)
	}

	fmt.Printf("[BİLGİ] İşlemler %s portu üzerinden yapılacak.\n", activePort)

	// 2) Ön bağlantı kontrolleri için Tor'a yönlenmiş HTTP istemcisi oluştur.
	fmt.Println("[İŞLEM] Tor HTTP client oluşturuluyor...")
	torClient, err := createTorClient(activePort)
	if err != nil {
		log.Fatal("[HATA❌] Client oluşturulamadı: ", err)
	}

	// 3) YAML konfigürasyonundan hedefleri yükler.
	config, err := loadConfig("sites.yaml")
	if err != nil {
		log.Fatal("[HATA❌] Konfig yüklenemedi: ", err)
	}
	if len(config.Site) == 0 {
		log.Fatal("[HATA❌] sites.yaml içinde hiç site yok.🥺")
	}

	// 4) Hedef seç, tara, sonucu logla ve menüye dön.
	reader := bufio.NewReader(os.Stdin)
	var lastErr error
	for {
		selectedIdx, exit, selErr := printMenuAndSelectSite(reader, config.Site, lastErr)
		if selErr != nil {
			fmt.Printf("[HATA❌] %v\n", selErr)
			lastErr = selErr
			continue
		}
		if exit {
			fmt.Println("Çıkış yapılıyor.👋")
			return
		}

		s := config.Site[selectedIdx]
		if err := scrapeSingleSite(torClient, activePort, s); err != nil {
			fmt.Printf("[HATA❌] %v\n", err)
			writeLog(fmt.Sprintf("BAŞARISIZ❌ | %s | %s | Hata: %v", s.Isim, s.Url, err))
			lastErr = err
			continue
		}

		fmt.Println("[BAŞARILI✅] Çıktılar results/ klasörüne kaydedildi.")
		writeLog(fmt.Sprintf("BAŞARILI✅ | %s | %s", s.Isim, s.Url))
		lastErr = nil
	}
}
