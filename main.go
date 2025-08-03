package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"time"

	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
	"golang.org/x/time/rate"
)

var (
	version              = ""
	printVersion         = flag.Bool("version", false, "print version")
	targetURL            = flag.String("url", "", "url address of target to send requests to")
	method               = flag.String("method", fasthttp.MethodGet, "HTTP method to use")
	delayBetweenRequests = flag.Duration("delay", 0, "delay between requests")
	maxGoroutines        = flag.Int("max_goroutines", 10, "limit of maximum goroutines count")
	requestTimeout       = flag.Duration("request_timeout", time.Second, "timeout for each request")
	logLevel             = flag.String("lvl", "info", "log level")
	userAgent            = flag.String("user_agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36", "user-agent used for requests")
	executionTime        = flag.Duration("exec_time", 0, "total duration of execution")
	prettyLog            = flag.Bool("pretty", false, "enable pretty logging")

	client  = &fasthttp.Client{}
	log     zerolog.Logger
	limiter *rate.Limiter
)

func main() {
	flag.Parse()
	if *printVersion {
		fmt.Println(version)
		return
	}
	*method = strings.ToUpper(*method)

	if *prettyLog {
		log = zerolog.New(zerolog.NewConsoleWriter())
	} else {
		log = zerolog.New(os.Stdout)
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	if *delayBetweenRequests != 0 {
		limiter = rate.NewLimiter(rate.Every(*delayBetweenRequests), 1)
	}

	lvl, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		log.Fatal().Timestamp().Err(err).Send()
	}

	zerolog.SetGlobalLevel(lvl)

	if _, err := url.Parse(*targetURL); err != nil {
		log.Fatal().Err(err).Timestamp().Str("url", *targetURL).Err(err).Msg("Invalid targetURL")
	}

	allowedHTTPMethods := []string{fasthttp.MethodGet, fasthttp.MethodPost, fasthttp.MethodPut, fasthttp.MethodPatch, fasthttp.MethodDelete, fasthttp.MethodHead, fasthttp.MethodOptions, fasthttp.MethodConnect, fasthttp.MethodTrace}

	switch {
	case *targetURL == "":
		log.Fatal().Timestamp().Msg("targetURL is required")
	case *delayBetweenRequests < 0:
		log.Fatal().Timestamp().Msg("delayBetweenRequests must be non-negative")
	case *maxGoroutines < 1:
		log.Fatal().Timestamp().Msg("maxGoroutines must be at least 1")
	case !slices.Contains(allowedHTTPMethods, *method):
		log.Fatal().Timestamp().Msg("invalid HTTP method")
	}

	sem := make(chan struct{}, *maxGoroutines)
	respChan := make(chan *Result, *maxGoroutines)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var sentRequestCount, errCount, totalDuration int64
	executionTimer := time.NewTimer(*executionTime)
	startedAt := time.Now()
	wg := &sync.WaitGroup{}

	templateReq := prepareRequest(*targetURL, *method, *userAgent)
	defer fasthttp.ReleaseRequest(templateReq)

	log.Info().Timestamp().Str("url", *targetURL).Msg("Sending requests to target")

	go func() {
		for {
			select {

			default:
				if limiter != nil {
					log.Debug().Timestamp().Err(limiter.Wait(ctx)).Send()
				}
				select {
				case sem <- struct{}{}:
					go sendRequest(ctx, sem, respChan, templateReq, *requestTimeout)
				case <-ctx.Done():
					return
				}

			case res := <-respChan:
				wg.Add(1)
				go processResponse(res, &errCount, &sentRequestCount, &totalDuration, wg)

			case <-executionTimer.C:
				if *executionTime != 0 {
					log.Debug().Timestamp().Msg("ExecutionTime reached, shutting down...")
					cancel()
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	<-ctx.Done()
	wg.Wait()

	var avgDuration float64
	if sentRequestCount > 0 {
		avgDuration = float64(sentRequestCount / totalDuration)
	}
	rps := float64(sentRequestCount) / time.Since(startedAt).Seconds()

	log.Info().Timestamp().Int64("sent_requests", sentRequestCount).Int64("errors", errCount).Float64("average_request_duration", avgDuration).Float64("requests_per_second", rps).Msg("Network throughput testing finished")
}

type Result struct {
	status   int
	err      error
	duration time.Duration
}

func sendRequest(ctx context.Context, sem <-chan struct{}, respChan chan<- *Result, templateReq *fasthttp.Request, requestTimeout time.Duration) {
	defer func() {
		select {
		case <-sem:
		case <-ctx.Done():
			return
		}
	}()

	start := time.Now()
	req := fasthttp.AcquireRequest()
	templateReq.CopyTo(req)

	resp := fasthttp.AcquireResponse()
	err := client.DoTimeout(req, resp, requestTimeout)

	res := &Result{
		status:   resp.StatusCode(),
		duration: time.Since(start),
		err:      err,
	}

	fasthttp.ReleaseRequest(req)
	fasthttp.ReleaseResponse(resp)

	select {
	case respChan <- res:
	case <-ctx.Done():
	}
}

func processResponse(res *Result, errCount, sentRequestsCount, totalDuration *int64, wg *sync.WaitGroup) {
	defer wg.Done()

	if res.err != nil {
		atomic.AddInt64(errCount, 1)
		log.Debug().Timestamp().Err(res.err).Send()
	}

	atomic.AddInt64(sentRequestsCount, 1)
	atomic.AddInt64(totalDuration, int64(res.duration))
	log.Debug().Timestamp().Int("status", res.status).Dur("duration", res.duration).Send()
}

func prepareRequest(url, method, userAgent string) *fasthttp.Request {
	req := fasthttp.AcquireRequest()
	req.SetRequestURI(url)
	req.Header.SetMethod(method)
	req.Header.SetUserAgent(userAgent)
	return req
}
