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
	// 0. Инициализация Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     "38.244.134.103:6380",
		Password: "asd12edasd112ad", 
		DB:       0,  
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Ошибка Redis: %v", err)
	}
	fmt.Println("🗄️  Redis подключен (localhost:6379)")

	// ТВОИ НОВЫЕ ПРОКСИ (ALMATY SESSION)
	proxyHost := "proxy.soax.com:5000"
	proxyUser := "package-341431-country-kz-city-almaty-sessionid-j6NEPCOsYoZDQjdL-sessionlength-300"
	proxyPass := "wk4W1L1IbaYEaKPC"
	proxyFull := fmt.Sprintf("http://%s:%s@%s", proxyUser, proxyPass, proxyHost)

	// 1. Настройка Rod через прокси
	l := launcher.New().
		Headless(true).
		NoSandbox(true).
		Proxy(proxyHost)

	launchURL, err := l.Launch()
	if err != nil {
		log.Fatal(err)
	}

	browser := rod.New().ControlURL(launchURL).MustConnect()
	defer browser.MustClose()
	
	// Авторизация прокси в браузере
	go browser.MustHandleAuth(proxyUser, proxyPass)()

	page := stealth.MustPage(browser)
	itemURL := "https://halykmarket.kz/category/smartfony/smartfon-apple-iphone-17-pro-6a58?sku=256gb_cosmicorange"

	fmt.Println("🌐 [SOAX ALMATY] Прогрев сессии...")
	if err := page.Navigate(itemURL); err != nil {
		fmt.Printf("⚠️ Ошибка навигации: %v\n", err)
	}
	
	fmt.Println("⏳ Проходим DDoS-Guard (30 сек)...")
	time.Sleep(30 * time.Second)
	page.Mouse.MustScroll(0, 500)

	// 2. HTTP Клиент для Go запросов
	pURL, _ := url.Parse(proxyFull)
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(pURL)},
		Timeout:   30 * time.Second,
	}

	for {
		fmt.Printf("\n--- [%s] ЗАПРОС ТОКЕНА ---\n", time.Now().Format("15:04:05"))

		// Собираем куки
		cookies, _ := page.Cookies([]string{})
		var cookieParts []string
		for _, c := range cookies {
			cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}
		cookieHeader := strings.Join(cookieParts, "; ")

		if len(cookieParts) < 5 {
			fmt.Println("⚠️  Сессия не готова, ждем...")
			page.MustReload()
			time.Sleep(20 * time.Second)
			continue
		}

		ua, _ := page.Eval(`() => navigator.userAgent`)

		// Делаем POST запрос
		req, _ := http.NewRequest("POST", "https://halykmarket.kz/submarkets/offer-service/public/offers/token", nil)
		req.Header.Set("accept", "application/json, text/plain, */*")
		req.Header.Set("citycode", "750000000")
		req.Header.Set("cookie", cookieHeader)
		req.Header.Set("user-agent", ua.Value.Str())
		req.Header.Set("origin", "https://halykmarket.kz")
		req.Header.Set("referer", itemURL)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("❌ Ошибка сети: %v\n", err)
			page.MustReload() // На всякий случай обновляем сессию
		} else {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			
			if resp.StatusCode == 200 {
    tokenData := string(body)
    fmt.Println("🎯 ТОКЕН ПОЛУЧЕН. Добавляю в список Redis...")

    // 1. Добавляем токен в начало списка (LPUSH)
    // Ключ: halyk:tokens_list
    err := rdb.LPush(ctx, "halyk:tokens_list", tokenData).Err()
    if err != nil {
        fmt.Printf("❌ Ошибка записи в список: %v\n", err)
    }

    // 2. Обрезаем список, чтобы в нем было не больше 20 свежих токенов (LTRIM)
    // rdb.LTrim(ctx, "halyk:tokens_list", 0, 19)

    // 3. Обновляем время жизни всего списка (5 минут)
    rdb.Expire(ctx, "halyk:tokens_list", 30000*time.Second)

    fmt.Println("✅ Токен добавлен в массив. Всего в базе: ~20 последних.")
} else {
				fmt.Printf("⚠️ Статус %d. Пробую восстановить сессию...\n", resp.StatusCode)
				page.MustReload()
				time.Sleep(20 * time.Second)
			}
		}

		time.Sleep(6 * time.Second)
	}
}
