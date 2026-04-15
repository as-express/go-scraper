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
	workerCount  = 200 
	
	proxyRawAddr = "res-unlimited-ef41714c.plainproxies.com:8080"
	proxyPass    = "anZI8uGb8WpS3rZ"
	proxyBaseUsr = "IPNz1S83Ei-country-kz"
)

var userAgents = []string{
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148",
	"Mozilla/5.0 (Linux; Android 14; SM-S928B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.6312.118 Mobile Safari/537.36",
	"Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Mobile Safari/537.36",
}

var globalTransport = &http.Transport{
	MaxIdleConns:        1000,
	MaxIdleConnsPerHost: 200,
	IdleConnTimeout:     90 * time.Second,
}

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
		log.Println("👑 [Master] Обновление сессии...")
		l := launcher.New().Bin("/usr/bin/google-chrome").Headless(true).NoSandbox(true)
		u, err := l.Launch()
		if err != nil {
			time.Sleep(10 * time.Second)
			continue
		}

		browser := rod.New().ControlURL(u).MustConnect().MustIgnoreCertErrors(true)
		go func() { _ = browser.HandleAuth(proxyBaseUsr+"-master-"+fmt.Sprint(rand.Intn(999)), proxyPass)() }()

		page := stealth.MustPage(browser)
		err = rod.Try(func() {
			page.MustNavigate(targets[rand.Intn(len(targets))])
			page.MustWaitIdle()
			time.Sleep(time.Duration(12+rand.Intn(5)) * time.Second)
		})

		cookies, _ := page.Cookies([]string{})
		var cookieParts []string
		for _, c := range cookies {
			cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}

		if len(cookieParts) >= 4 {
			shared.mu.Lock()
			shared.Cookie = strings.Join(cookieParts, "; ")
			shared.mu.Unlock()
			log.Printf("✅ [Master] Сессия готова. (Кук: %d)", len(cookieParts))
			browser.MustClose()
			time.Sleep(120 * time.Second) 
		} else {
			browser.MustClose()
			time.Sleep(10 * time.Second)
		}
	}
}

func runWorker(id int, rdb *redis.Client) {
	client := &http.Client{Transport: globalTransport, Timeout: 10 * time.Second}
	myUA := userAgents[rand.Intn(len(userAgents))]

	for testRunning {
		shared.mu.RLock()
		cookie := shared.Cookie
		shared.mu.RUnlock()

		if cookie == "" {
			time.Sleep(2 * time.Second)
			continue
		}

		sessionID := rand.Intn(1000000)
		proxyStr := fmt.Sprintf("http://%s-session-%d-ttl-60:%s@%s", proxyBaseUsr, sessionID, proxyPass, proxyRawAddr)
		proxyURL, _ := url.Parse(proxyStr)
		client.Transport.(*http.Transport).Proxy = http.ProxyURL(proxyURL)

		target := targets[rand.Intn(len(targets))]
		city := cities[rand.Intn(len(cities))]

		req, _ := http.NewRequest("POST", "https://m.halykmarket.kz/submarkets/offer-service/public/offers/token", nil)
		req.Header.Set("User-Agent", myUA)
		req.Header.Set("Cookie", cookie)
		req.Header.Set("CityCode", city)
		req.Header.Set("Referer", target)
		req.Header.Set("Accept", "application/json")
		
		req.Header.Set("Sec-Fetch-Dest", "empty")
		req.Header.Set("Sec-Fetch-Mode", "cors")
		req.Header.Set("Sec-Fetch-Site", "same-origin")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			
			if strings.Contains(string(body), "offersToken") {
				rdb.ZAdd(ctx, "halyk:tokens", redis.Z{Score: float64(time.Now().Unix()), Member: string(body)})
				
				counterMu.Lock()
				if startTime.IsZero() { startTime = time.Now() }
				totalSaved++
				counterMu.Unlock()
				
				jitter := 150 + rand.Intn(350) 
				time.Sleep(time.Duration(jitter) * time.Millisecond)

				if rand.Intn(100) < 5 {
					time.Sleep(time.Duration(3+rand.Intn(4)) * time.Second)
				}
			}
		} else {
			resp.Body.Close()
			time.Sleep(time.Duration(2+rand.Intn(3)) * time.Second)
		}
	}
}

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr, Password: redisPass, PoolSize: 500})
	go runMaster()

	for {
		shared.mu.RLock()
		ready := shared.Cookie != ""
		shared.mu.RUnlock()
		if ready { break }
		time.Sleep(2 * time.Second)
	}

	fmt.Printf("🚀 Безопасный запуск. Целей: %d | Воркеров: %d\n", len(targets), workerCount)
	for i := 1; i <= workerCount; i++ {
		go runWorker(i, rdb)
	}

	for testRunning {
		time.Sleep(30 * time.Second)
		if !startTime.IsZero() {
			counterMu.Lock()
			count := totalSaved
			counterMu.Unlock()
			elapsed := time.Since(startTime).Seconds()
			log.Printf("📊 СТАТ: %d токенов | Скорость: %.2f т/сек", count, float64(count)/elapsed)
		}
	}
}