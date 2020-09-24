package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"strconv"
	"strings"

	"github.com/valyala/fasthttp"

	mcpinger "github.com/Raqbit/mc-pinger"
)

var servers = make(map[string]mcpinger.Pinger)
var keysBytes []byte

type serben struct {
	Online uint `json:"online"`
	Max    uint `json:"max"`
}

func init() {
	bytesServers, err := ioutil.ReadFile("servers.json")
	if err != nil {
		log.Fatal(err)
	}

	var tmp map[string]string
	if err := json.Unmarshal(bytesServers, &tmp); err != nil {
		log.Fatal(err)
	}

	keys := make([]string, 0, len(tmp))
	for k, v := range tmp {
		s := strings.Split(v, ":")
		if len(s) != 2 { //nolint: go-mnd
			log.Fatal("Malformed serben: ", s)
		}

		port, err := strconv.Atoi(s[1])
		if err != nil {
			log.Fatal("Invalid port: ", s[1])
		}

		log.Println("Found serben ", k, " with ip ", s[0], " on port ", port)

		servers[k] = mcpinger.New(s[0], uint16(port))
		keys = append(keys, k)
	}
	keysBytes, _ = json.Marshal(keys)
}

func main() {
	if err := fasthttp.ListenAndServe(":8001", fastHTTPHandler); err != nil {
		log.Fatalf("Error in ListenAndServe: %s", err)
	}
}

func fastHTTPHandler(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.SetContentType("application/json")
	ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
	ctx.Response.Header.Set("Access-Control-Allow-Methods", "GET")
	ctx.Response.Header.Set("X-Frame-Options", "DENY")
	ctx.Response.Header.Set("X-Content-Type-Options", "nosniff")
	ctx.Response.Header.Set("X-XSS-Protection", "1; mode=block")
	ctx.Response.Header.Set("Referrer-Policy", "no-referrer")
	ctx.Response.Header.Set("Content-Security-Policy", "default-src 'none';")

	path := ctx.Path()

	if bytes.Equal(path, []byte("/servers")) {
		listHandler(ctx)
		return
	}

	if bytes.HasPrefix(path, []byte("/server/")) {
		viewHandler(ctx)
		return
	}

	ctx.Error("Unsupported path", fasthttp.StatusNotFound)
}

func listHandler(ctx *fasthttp.RequestCtx) {
	ctx.Response.SetBody(keysBytes)
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
}

func viewHandler(ctx *fasthttp.RequestCtx) {
	k := ctx.Request.RequestURI()[len("/server/"):]
	if pinger, ok := servers[string(k)]; ok {
		info, _ := pinger.Ping()
		if info == nil {
			ctx.Response.Header.SetStatusCode(fasthttp.StatusServiceUnavailable)
			return
		}
		serv := serben{info.Players.Online, info.Players.Max}

		s, _ := json.Marshal(serv)
		ctx.Response.SetBody(s)
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		return
	}

	ctx.Error("Unsupported path", fasthttp.StatusNotFound)
}
