package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

func main() {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "38.244.134.103:6380",
		Password: "asd12edasd112ad",
		DB:       0,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Ошибка Redis: %v", err)
	}

	// Запускаем два воркера параллельно
	go tokenWorker(rdb, "worker-1", "session_A1")
	go tokenWorker(rdb, "worker-2", "session_B2")

	// Чтобы main не закрылся
	select {}
}

func tokenWorker(rdb *redis.Client, workerName string, sessionID string) {
	proxyHost := "proxy.soax.com:5000"
	// Динамически меняем sessionid в логине прокси
	proxyUser := fmt.Sprintf("package-341431-country-kz-city-almaty-sessionid-%s-sessionlength-300", sessionID)
	proxyPass := "wk4W1L1IbaYEaKPC"
	proxyFull := fmt.Sprintf("http://%s:%s@%s", proxyUser, proxyPass, proxyHost)

	l := launcher.New().
		Headless(true).
		NoSandbox(true).
		Proxy(proxyHost).
		Set("disable-gpu").
		Set("disable-dev-shm-usage")

	launchURL, err := l.Launch()
	if err != nil {
		log.Printf("[%s] Fatal launcher: %v", workerName, err)
		return
	}

	browser := rod.New().ControlURL(launchURL).MustConnect()
	defer browser.MustClose()
	go browser.MustHandleAuth(proxyUser, proxyPass)()

	page := stealth.MustPage(browser)
	itemURL := "https://halykmarket.kz/category/smartfony/smartfon-apple-iphone-17-pro-6a58?sku=256gb_cosmicorange"

	fmt.Printf("🌐 [%s] Прогрев сессии (%s)...\n", workerName, sessionID)
	page.Navigate(itemURL)
	time.Sleep(30 * time.Second) // Ждем DDoS-Guard

	pURL, _ := url.Parse(proxyFull)
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(pURL)},
		Timeout:   30 * time.Second,
	}

	for {
		fmt.Printf("--- [%s | %s] ЗАПРОС ТОКЕНА ---\n", workerName, time.Now().Format("15:04:05"))

		cookies, _ := page.Cookies([]string{})
		var cookieParts []string
		for _, c := range cookies {
			cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}
		
		if len(cookieParts) < 5 {
			page.MustReload()
			time.Sleep(20 * time.Second)
			continue
		}

		cookieHeader := strings.Join(cookieParts, "; ")
		ua, _ := page.Eval(`() => navigator.userAgent`)

		req, _ := http.NewRequest("POST", "https://halykmarket.kz/submarkets/offer-service/public/offers/token", nil)
		req.Header.Set("citycode", "750000000")
		req.Header.Set("cookie", cookieHeader)
		req.Header.Set("user-agent", ua.Value.Str())
		req.Header.Set("referer", itemURL)

		resp, err := client.Do(req)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode == 200 {
				rdb.LPush(ctx, "halyk:tokens_list", string(body))
				rdb.Expire(ctx, "halyk:tokens_list", 300*time.Second)
				fmt.Printf("✅ [%s] Токен добавлен\n", workerName)
			}
			resp.Body.Close()
		}

		// Пауза чуть больше, чтобы не забанили оба потока сразу
		time.Sleep(8 * time.Second)
	}
}