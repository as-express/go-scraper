package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

const (
	redisAddr    = "localhost:6379"
	workerCount  = 10
	targetURL    = "https://halykmarket.kz/category/smartfony/smartfon-apple-iphone-17-pro-max-0e6f?sku=256gb_silver"
	testDuration = 5 * time.Minute
)

type SharedData struct {
	Cookie string
	DTPC   string
	UA     string
	mu     sync.RWMutex
}

var (
	shared      = &SharedData{}
	totalSaved  int64
	counterMu   sync.Mutex
	startTime   time.Time
	timerOnce   sync.Once
	testRunning = true
)

func runMaster() {
	for {
		log.Println("👑 [Master] Обновление сессии через Real Chrome...")
		l := launcher.New().Headless(false).Devtools(false)
		u, err := l.Launch()
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

		browser := rod.New().ControlURL(u).MustConnect()
		page := stealth.MustPage(browser)

		if err := page.Navigate(targetURL); err != nil {
			browser.MustClose()
			continue
		}

		time.Sleep(15 * time.Second)
		page.Mouse.MustScroll(0, 400)
		time.Sleep(5 * time.Second)

		cookies, _ := page.Cookies([]string{})
		var cookieParts []string
		dtpc := ""
		for _, c := range cookies {
			cookieParts = append(cookieParts, fmt.Sprintf("%s=%s", c.Name, c.Value))
			if c.Name == "dtPC" {
				dtpc = c.Value
			}
		}
		fullCookie := strings.Join(cookieParts, "; ")

		if len(fullCookie) > 1500 && dtpc != "" {
			ua, _ := page.Eval(`() => navigator.userAgent`)
			shared.mu.Lock()
			shared.Cookie = fullCookie
			shared.DTPC = dtpc
			shared.UA = ua.Value.Str()
			shared.mu.Unlock()
			log.Printf("✅ [Master] СЕССИЯ ГОТОВА. DTPC: %s", dtpc)
			time.Sleep(10 * time.Minute)
		} else {
			time.Sleep(5 * time.Second)
		}
		browser.MustClose()
	}
}

func runWorker(id int, rdb *redis.Client) {
	l := launcher.New().Headless(true)
	u, _ := l.Launch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	for testRunning {
		shared.mu.RLock()
		cookie, ua, dtpc := shared.Cookie, shared.UA, shared.DTPC
		shared.mu.RUnlock()

		if dtpc == "" {
			time.Sleep(2 * time.Second)
			continue
		}

		page := stealth.MustPage(browser)
		_ = page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: ua})

		for _, p := range strings.Split(cookie, "; ") {
			kv := strings.SplitN(p, "=", 2)
			if len(kv) == 2 {
				_ = page.SetCookies([]*proto.NetworkCookieParam{{
					Name: kv[0], Value: kv[1], Domain: "halykmarket.kz",
				}})
			}
		}

		if err := page.Navigate(targetURL); err != nil {
			page.MustClose()
			continue
		}
		page.MustWaitStable()

		for i := 0; i < 10 && testRunning; i++ {
			js := fmt.Sprintf(`async () => {
				try {
					const res = await fetch("https://halykmarket.kz/submarkets/offer-service/public/offers/token", {
						method: "POST",
						headers: { 
							"accept": "application/json, text/plain, */*",
							"citycode": "750000000",
							"x-dtpc": "%s"
						}
					});
					return await res.text();
				} catch (e) { return "ERR"; }
			}`, dtpc)

			result, err := page.Eval(js)
			resBody := ""
			if err == nil {
				resBody = result.Value.Str()
			}

			if strings.Contains(resBody, "offersToken") {
				// Запуск таймера при ПЕРВОМ токене
				timerOnce.Do(func() {
					startTime = time.Now()
					log.Println("🏁 ТЕСТ НАЧАТ! ОТСЧЕТ 5 МИНУТ ПОШЕЛ...")
					go func() {
						time.Sleep(testDuration)
						testRunning = false
						log.Println("🛑 ВРЕМЯ ВЫШЛО!")
					}()
				})

				rdb.ZAdd(ctx, "halyk:tokens", redis.Z{Score: float64(time.Now().Unix()), Member: resBody})
				counterMu.Lock()
				totalSaved++
				counterMu.Unlock()
				log.Printf("🔥 [W%d] +1 (Всего: %d)", id, totalSaved)
			} else {
				log.Printf("❌ [W%d] Ошибка API, сплю 10 сек...", id)
				time.Sleep(10 * time.Second)
				break
			}
			time.Sleep(time.Duration(1500+rand.Intn(2000)) * time.Millisecond)
		}
		page.MustClose()
	}
}

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	go runMaster()

	fmt.Println("⏳ Ожидание инициализации...")
	for {
		shared.mu.RLock()
		ready := shared.DTPC != ""
		shared.mu.RUnlock()
		if ready { break }
		time.Sleep(2 * time.Second)
	}

	for i := 1; i <= workerCount; i++ {
		go runWorker(i, rdb)
	}

	for testRunning {
		time.Sleep(10 * time.Second)
		if !startTime.IsZero() {
			timeLeft := testDuration - time.Since(startTime)
			if timeLeft < 0 { timeLeft = 0 }
			log.Printf("📊 СТАТУС: %d токенов. Осталось: %v", totalSaved, timeLeft.Round(time.Second))
		}
	}

	fmt.Printf("\n🏆 ТЕСТ ЗАВЕРШЕН!\nИТОГО СОБРАНО: %d\nСРЕДНЯЯ СКОРОСТЬ: %.2f токенов/мин\n", 
		totalSaved, float64(totalSaved)/5.0)
}