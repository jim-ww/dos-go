# DoS-Go

A high-performance HTTP load testing tool written in Go, designed for network throughput testing and benchmarking web servers. Built with [fasthttp](https://github.com/valyala/fasthttp) and [zerolog](https://github.com/rs/zerolog) for maximum efficiency and speed.

## Features

- **Minimal** (under 200 LoC)
- **Blazing fast** - Utilizes fasthttp's optimized HTTP client implementation
- **Lightweight logging** - Zerolog provides minimal-overhead structured logging
- Multi-threaded requests with goroutine pooling
- Configurable request rate limiting
- Support for common HTTP methods (GET, POST, PUT, etc.)
- Detailed statistics collection (requests per second, error rate, avg. duration)

```bash
# Send requests to http://localhost:8080 for 10 seconds with 10,000 goroutines
$ dos -url http://localhost:8080 -exec_time 10s -max_goroutines 10000

# Send requests with 1 second delay between each request
$ dos -url http://localhost:8080 -delay 1s

# Enable debug logs with pretty formatting (NOTE: logging impacts performance)
$ dos -url http://localhost:8080 -lvl debug -pretty
```

## Usage

`$ dos -url <target_url> [flags]`

### Required Parameters

- `-url` - Target URL to test (e.g., `http://example.com`)

### Optional Parameters

- `-method` - HTTP method (default: `GET`)

- `-delay` - Delay between requests (e.g., `100ms`, `2s`)

- `-max_goroutines` - Maximum concurrent goroutines (default: `10`)

- `-request_timeout` - Timeout per request (default: `1s`)

- `-lvl` - Log level (debug, info, warn, error, fatal, panic) (default: `info`)

- `-user_agent` - Custom User-Agent string (default: `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36`)

- `-exec_time` - Total execution duration (e.g., `30s`, `5m`)

- `-pretty` - Enable pretty-printed logs (default: `false`)

## Building from source

To build from source, you will need to have Go (1.24+) installed on your system. Once you have Go installed, you can clone the repository and build the binary using the following commands:

```bash
git clone https://github.com/jim-ww/dos-go.git
cd dos-go
go build -o dos
```

## License

[GPL-3.0](https://github.com/jim-ww/dos-go/blob/main/LICENSE)
