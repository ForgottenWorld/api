package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/valyala/fasthttp"

	mcpinger "github.com/Raqbit/mc-pinger"
)

var servers = make(map[string]mcpinger.Pinger)
var serverList []byte

var apiKey string
var panel string

var lastRefresh time.Time

func init() {
	apiKey = os.Getenv("PTERO_API_KEY")
	panel = os.Getenv("PANEL")

	if err := refresh(); err != nil {
		log.Fatalf("Error in init: %s", err)
	}
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

	if bytes.Equal(path, []byte("/")) {
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		return
	}

	if bytes.Equal(path, []byte("/servers")) {
		listHandler(ctx)
		return
	}

	if bytes.Equal(path, []byte("/refresh")) {
		refreshHandler(ctx)
		return
	}

	if bytes.HasPrefix(path, []byte("/server/")) {
		viewHandler(ctx)
		return
	}

	ctx.Error("Unsupported path", fasthttp.StatusNotFound)
}

func listHandler(ctx *fasthttp.RequestCtx) {
	ctx.Response.SetBody(serverList)
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
}

func refreshHandler(ctx *fasthttp.RequestCtx) {
	elapsed := time.Since(lastRefresh)
	if elapsed.Seconds() < 10 { //nolint: go-mnd
		ctx.Response.SetBody([]byte("[\"Cached\"]"))
		ctx.Response.SetStatusCode(fasthttp.StatusFound)
		return
	}
	lastRefresh = time.Now()
	if err := refresh(); err != nil {
		log.Printf("Error in refreshHandler: %s", err)
		ctx.Response.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}
	ctx.Response.SetBody([]byte("[\"OK\"]"))
	ctx.Response.SetStatusCode(fasthttp.StatusOK)
}

func viewHandler(ctx *fasthttp.RequestCtx) {
	k := ctx.Request.RequestURI()[len("/server/"):]
	if pinger, ok := servers[string(k)]; ok {
		info, err := pinger.Ping()
		if err != nil {
			log.Println(err)
			ctx.Response.Header.SetStatusCode(fasthttp.StatusServiceUnavailable)
			return
		}
		if info == nil {
			ctx.Response.Header.SetStatusCode(fasthttp.StatusServiceUnavailable)
			return
		}
		serv := struct {
			Online uint `json:"online"`
			Max    uint `json:"max"`
		}{info.Players.Online, info.Players.Max}

		s, _ := json.Marshal(serv)
		ctx.Response.SetBody(s)
		ctx.Response.SetStatusCode(fasthttp.StatusOK)
		return
	}

	ctx.Error("Unsupported path", fasthttp.StatusNotFound)
}

func refresh() error {
	ss, err := getServers()
	if err != nil {
		return err
	}
	jrsp := struct {
		Data []struct {
			Attributes struct {
				Name        string
				Description string
				Allocation  int
				Node        int
			} `json:"attributes"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(ss, &jrsp); err != nil {
		return err
	}

	keys := make([]string, 0, len(jrsp.Data))
	nodes := make(map[int]map[int]mcpinger.Pinger, len(jrsp.Data))
	for _, s := range jrsp.Data {
		if !strings.Contains(s.Attributes.Description, "[api=true]") {
			log.Printf("Skipping server: %s", s.Attributes.Name)
			continue
		}
		if _, ok := nodes[s.Attributes.Node]; !ok {
			allocs, err := getAllocations(s.Attributes.Node)
			if err != nil {
				return err
			}
			nodes[s.Attributes.Node] = allocs
		}

		if pinger, ok := nodes[s.Attributes.Node][s.Attributes.Allocation]; ok {
			servers[s.Attributes.Name] = pinger
			keys = append(keys, s.Attributes.Name)
		} else {
			log.Printf("Missing allocs for server: %s", s.Attributes.Name)
		}
	}

	serverList, err = json.Marshal(keys)
	return err
}

func getServers() ([]byte, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(panel + "api/application/servers")
	req.Header.SetContentType("application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	err := fasthttp.Do(req, resp)
	if err != nil {
		return []byte{}, err
	}

	return resp.Body(), nil
}

func getAllocations(node int) (map[int]mcpinger.Pinger, error) {
	jrsp := struct {
		Data []struct {
			Attributes struct {
				ID       int
				IP       string
				Port     uint16
				Assigned bool
			} `json:"attributes"`
		} `json:"data"`
	}{}
	b, err := getAllocs(node)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(b, &jrsp); err != nil {
		return nil, err
	}

	allocs := make(map[int]mcpinger.Pinger)
	for _, a := range jrsp.Data {
		if !a.Attributes.Assigned {
			log.Printf("Skipping unassigned alloc %d at %s:%d", a.Attributes.ID, a.Attributes.IP, a.Attributes.Port)
			continue
		}
		log.Printf("Found alloc %d at %s:%d", a.Attributes.ID, a.Attributes.IP, a.Attributes.Port)
		allocs[a.Attributes.ID] = mcpinger.New(a.Attributes.IP, a.Attributes.Port)
	}

	return allocs, nil
}

func getAllocs(node int) ([]byte, error) {
	n := strconv.Itoa(node)
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(panel + "api/application/nodes/" + n + "/allocations")
	req.Header.SetContentType("application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	err := fasthttp.Do(req, resp)
	if err != nil {
		return []byte{}, err
	}

	return resp.Body(), nil
}
