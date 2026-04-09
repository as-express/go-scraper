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
	redisAddr = "38.244.134.103:6380"
	redisPass = "asd12edasd112ad"
	proxyHost = "res-unlimited-ef41714c.plainproxies.com:8080"
	proxyPass = "9LYOsXqbaVmRpEj"
)

type Session struct {
	Cookie string
	UA     string
	mu     sync.RWMutex
}

func runWorker(id int, user string, itemURL string, rdb *redis.Client) {
	fmt.Printf("🚀 [Worker %d] Starting...\n", id)

	l := launcher.New().Headless(true).NoSandbox(true).Proxy(proxyHost)
	launchURL, err := l.Launch()
	if err != nil {
		log.Printf("❌ [Worker %d] Launcher Error: %v", id, err)
		return
	}

	browser := rod.New().ControlURL(launchURL).MustConnect()
	defer browser.MustClose()

	go browser.MustHandleAuth(user, proxyPass)()
	page := stealth.MustPage(browser)

	session := &Session{}

	updateSession := func() {
		session.mu.Lock()
		defer session.mu.Unlock()

		cookies, _ := page.Cookies([]string{})
		var cookieParts []string
		for _, c := range cookies {
			cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}
		session.Cookie = strings.Join(cookieParts, "; ")
		ua, _ := page.Eval(`() => navigator.userAgent`)
		session.UA = ua.Value.Str()
	}

	if err := page.Navigate(itemURL); err != nil {
		log.Printf("❌ [Worker %d] Init Nav Error: %v", id, err)
		return
	}
	time.Sleep(20 * time.Second)
	page.Mouse.MustScroll(0, 700)
	updateSession()

	proxyFull := fmt.Sprintf("http://%s:%s@%s", user, proxyPass, proxyHost)
	pURL, _ := url.Parse(proxyFull)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(pURL),
		},
		Timeout: 15 * time.Second,
	}

	go func() {
		for {
			qLen, err := rdb.LLen(ctx, "halyk:tokens_list").Result()
			if err == nil && qLen >= 200 {
				time.Sleep(10 * time.Second)
				continue
			}

			session.mu.RLock()
			cookieHeader := session.Cookie
			ua := session.UA
			session.mu.RUnlock()

			if len(cookieHeader) < 20 {
				time.Sleep(2 * time.Second)
				continue
			}

			req, _ := http.NewRequest("POST", "https://halykmarket.kz/submarkets/offer-service/public/offers/token", nil)
			req.Header.Set("accept", "application/json, text/plain, */*")
			req.Header.Set("citycode", "750000000")
			req.Header.Set("cookie", cookieHeader)
			req.Header.Set("user-agent", ua)
			req.Header.Set("origin", "https://halykmarket.kz")
			req.Header.Set("referer", itemURL)

			resp, err := client.Do(req)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode == 200 {
				rdb.LPush(ctx, "halyk:tokens_list", string(body))
				rdb.Expire(ctx, "halyk:tokens_list", 24*time.Hour)
			} else if resp.StatusCode == 429 {
				log.Printf("⚠️ [Worker %d] Rate limit! Sleeping 30s...", id)
				time.Sleep(30 * time.Second)
			}

			sleepMs := 300 + rand.Intn(200)
			time.Sleep(time.Duration(sleepMs) * time.Millisecond)
		}
	}()

	for {
		time.Sleep(15 * time.Minute)
		if err := page.Reload(); err != nil {
			log.Printf("⚠️ [Worker %d] Reload failed: %v", id, err)
			continue
		}
		time.Sleep(10 * time.Second)
		updateSession()
	}
}

func main() {
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPass,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Redis connection failed: %v", err)
	}
	fmt.Println("🗄️ Redis подключен")

	workersCount := 3 

	for i := 1; i <= workersCount; i++ {
		sessionID := fmt.Sprintf("session-%d", i)
		proxyUser := "T8uC7pgTQX-country-kz-city-almaty-" + sessionID
		
		go runWorker(i, proxyUser, 
			"https://halykmarket.kz/category/smartfony/smartfon-apple-iphone-17-pro-6a58?sku=256gb_cosmicorange", rdb)
		
		time.Sleep(8 * time.Second) 
	}

	select {}
}