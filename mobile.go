package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

const (
	redisAddr    = "89.207.255.53:6380"
	redisPass    = "asd12edasd112ad"
	workerCount  = 50
	testDuration = 30 * time.Minute

	proxyRawAddr = "res-unlimited-ef41714c.plainproxies.com:8080"
	proxyPass    = "anZI8uGb8WpS3rZ"
	proxyBaseUsr = "IPNz1S83Ei-country-kz"
)

var cities = []string{"750000000", "710000000", "511010000"}

var targets = []string{
	"https://m.halykmarket.kz/category/smartfony/smartfon-xiaomi-redmi-15-3298?sku=8256gb_sandypurple",
	"https://m.halykmarket.kz/category/smartfony/smartfon-xiaomi-redmi-15-3298?sku=8256gb_midnightblack",
	"https://m.halykmarket.kz/category/smartfony/smartfon-xiaomi-redmi-15-3298?sku=8256gb_titangray",
	"https://m.halykmarket.kz/category/smartfony/smartfon-poco-c71-5401?sku=4128gb_chernyy",
	"https://m.halykmarket.kz/category/smartfony/smartfon-redmi-15c-nfc-12e6?sku=4128gb_black",
	"https://m.halykmarket.kz/category/smartfony/smartfon-apple-iphone-17-pro-6a58?sku=512gb_cosmicorange",
	"https://m.halykmarket.kz/category/smartfony/smartfon-apple-iphone-17-pro-6a58?sku=512gb_silver",
	"https://m.halykmarket.kz/category/smartfony/smartfon-samsung-galaxy-a26-6128gb-400e?sku=black",
	"https://m.halykmarket.kz/category/smartfony/smartfon-xiaomi-redmi-a5-166a?sku=4128gb_green",
	"https://m.halykmarket.kz/category/smartfony/smartfon-xiaomi-redmi-a5-166a?sku=4128gb_blue",
	"https://m.halykmarket.kz/category/smartfony/smartfon-samsung-galaxy-z-flip-7-f54a?sku=12512gb_blueshadow",
	"https://m.halykmarket.kz/category/smartfony/smartfon-samsung-galaxy-z-flip-7-f54a?sku=12512gb_mint",
	"https://m.halykmarket.kz/category/smartfony/smartfon-samsung-galaxy-s25-ultra?sku=12256gb_titaniumblue",
	"https://m.halykmarket.kz/category/smartfony/smartfon-samsung-galaxy-s25-ultra?sku=12256gb_titaniumblack",
	"https://m.halykmarket.kz/category/smartfony/smartfon-apple-iphone-17-faf2?sku=256gb_mistblue",
	"https://m.halykmarket.kz/category/smartfony/smartfon-apple-iphone-17-faf2?sku=256gb_white",
	"https://m.halykmarket.kz/category/smartfony/smartfon-apple-iphone-17-pro-max-0e6f?sku=256gb_silver",
	"https://m.halykmarket.kz/category/smartfony/smartfon-samsung-galaxy-a07-3ad9?sku=6128gb_black",
	"https://m.halykmarket.kz/category/smartfony/smartfon-huawei-nova-14-89b5?sku=512gb_white",
	"https://m.halykmarket.kz/category/igrovie-pristavki/igrovaja-pristavka-sony-playstation-5-slim-belyj",
	"https://m.halykmarket.kz/category/igrovie-pristavki/nintendo-switch-oled-belyy-",
}

type SharedData struct {
	Cookie string
	UA     string
	mu     sync.RWMutex
}

var (
	shared      = &SharedData{}
	totalSaved  int64
	counterMu   sync.Mutex
	startTime   time.Time
	testRunning = true
)

func runMaster() {
	for {
		log.Println("👑 [Master] Обновление глобальной сессии...")
		l := launcher.New().Headless(true)
		u, err := l.Launch()
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		browser := rod.New().ControlURL(u).MustConnect().MustIgnoreCertErrors(true)
		go func() { _ = browser.HandleAuth(proxyBaseUsr+"-session-master", proxyPass)() }()

		page := stealth.MustPage(browser)
		err = rod.Try(func() {
			page.MustNavigate(targets[0])
			page.MustWaitIdle()
			time.Sleep(15 * time.Second)
		})

		cookies, _ := page.Cookies([]string{})
		var cookieParts []string
		for _, c := range cookies {
			cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}

		if len(cookieParts) > 5 {
			shared.mu.Lock()
			shared.Cookie = strings.Join(cookieParts, "; ")
			shared.UA = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148"
			shared.mu.Unlock()
			log.Printf("✅ [Master] Сессия готова. Кук: %d", len(cookieParts))
			browser.MustClose()
			time.Sleep(7 * time.Minute)
		} else {
			browser.MustClose()
			time.Sleep(10 * time.Second)
		}
	}
}

func runWorker(id int, rdb *redis.Client) {
	for testRunning {
		sessionID := rand.Intn(999999)
		proxyStr := fmt.Sprintf("http://%s-session-%d-ttl-600:%s@%s", proxyBaseUsr, sessionID, proxyPass, proxyRawAddr)
		proxyURL, _ := url.Parse(proxyStr)

		client := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				MaxIdleConns: 5,
				IdleConnTimeout: 30 * time.Second,
			},
			Timeout: 10 * time.Second,
		}

		shared.mu.RLock()
		cookie, ua := shared.Cookie, shared.UA
		shared.mu.RUnlock()

		if cookie == "" {
			time.Sleep(2 * time.Second)
			continue
		}

		target := targets[rand.Intn(len(targets))]
		city := cities[rand.Intn(len(cities))]

		req, _ := http.NewRequest("POST", "https://m.halykmarket.kz/submarkets/offer-service/public/offers/token", nil)
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Cookie", cookie)
		req.Header.Set("CityCode", city)
		req.Header.Set("Referer", target)
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Content-Length", "0")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			resBody := string(body)

			if strings.Contains(resBody, "offersToken") {
				rdb.ZAdd(ctx, "halyk:tokens", redis.Z{Score: float64(time.Now().Unix()), Member: resBody})
				counterMu.Lock()
				if startTime.IsZero() { startTime = time.Now() }
				totalSaved++
				counterMu.Unlock()
				
				time.Sleep(time.Duration(2500+rand.Intn(3000)) * time.Millisecond) // Чуть ускорил интервал
			}
		} else {
			resp.Body.Close()
			time.Sleep(5 * time.Second)
		}
	}
}

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr, Password: redisPass})
	go runMaster()

	fmt.Println("🚀 Запуск мульти-таргет + мульти-сити воркеров...")
	for {
		shared.mu.RLock()
		if shared.Cookie != "" { 
			shared.mu.RUnlock()
			break 
		}
		shared.mu.RUnlock()
		time.Sleep(1 * time.Second)
	}

	for i := 1; i <= workerCount; i++ {
		go runWorker(i, rdb)
	}

	for testRunning {
		time.Sleep(300 * time.Second)
		if !startTime.IsZero() {
			log.Printf("📊 СТАТ: %d токенов | Целей: %d | Городов: 2 | Скорость: %.2f т/сек", 
				totalSaved, len(targets), float64(totalSaved)/time.Since(startTime).Seconds())
		}
	}
}