package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cache"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	mcpinger "github.com/Raqbit/mc-pinger"
)

var servers = make(map[string]mcpinger.Pinger)
var serverList []string

var apiKey string
var panel string

func init() {
	apiKey = os.Getenv("PTERO_API_KEY")
	panel = os.Getenv("PANEL")

	if err := refresh(); err != nil {
		log.Fatalf("Error in init: %s", err)
	}
}

func main() {
	app := fiber.New(fiber.Config{
		UnescapePath: true,
	})
	app.Use(cors.New())
	app.Use(compress.New(compress.Config{Level: compress.LevelBestCompression}))
	app.Use(limiter.New(limiter.Config{
		Next: func(c *fiber.Ctx) bool {
			return c.Path() != "/reload"
		},
		Max:        1,
		Expiration: 10 * time.Second,
		KeyGenerator: func(c *fiber.Ctx) string {
			return "key"
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.SendStatus(fiber.StatusTooManyRequests)
		},
	}))
	app.Use(cache.New(cache.Config{
		Next: func(c *fiber.Ctx) bool {
			return c.Path() == "/reload"
		},
		Expiration: 3 * time.Second,
	}))

	app.Get("/servers", list)
	app.Get("/reload", reload)
	app.Get("/server/:name", view)
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	app.Listen(":8001")
}

func list(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(serverList)
}

func reload(c *fiber.Ctx) error {
	if err := refresh(); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	return c.SendStatus(fiber.StatusOK)
}

func view(c *fiber.Ctx) error {
	name := c.Params("name")
	if pinger, ok := servers[name]; ok {
		info, err := pinger.Ping()
		if err != nil {
			return c.SendStatus(fiber.StatusServiceUnavailable)
		}
		serv := struct {
			Online uint `json:"online"`
			Max    uint `json:"max"`
		}{info.Players.Online, info.Players.Max}
		return c.Status(fiber.StatusOK).JSON(serv)
	}
	return c.SendStatus(fiber.StatusNotFound)
}

func refresh() error {
	ss, err := loadServers()
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
			allocs, err := loadAllocations(s.Attributes.Node)
			if err != nil {
				return err
			}
			nodes[s.Attributes.Node] = allocs
		}

		if pinger, ok := nodes[s.Attributes.Node][s.Attributes.Allocation]; ok {
			log.Printf("Found server: %s", s.Attributes.Name)
			servers[s.Attributes.Name] = pinger
			keys = append(keys, s.Attributes.Name)
		} else {
			log.Printf("Missing allocs for server: %s", s.Attributes.Name)
		}
	}

	serverList = keys
	return err
}

func loadServers() ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, panel+"api/application/servers", nil)
	req.Header.Set("Content-type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return []byte{}, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}
	return body, nil
}

func loadAllocations(node int) (map[int]mcpinger.Pinger, error) {
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
	b, err := loadAllocs(node)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(b, &jrsp); err != nil {
		return nil, err
	}

	allocs := make(map[int]mcpinger.Pinger)
	for _, a := range jrsp.Data {
		if !a.Attributes.Assigned {
			continue
		}
		allocs[a.Attributes.ID] = mcpinger.New(a.Attributes.IP, a.Attributes.Port)
	}
	return allocs, nil
}

func loadAllocs(node int) ([]byte, error) {
	n := strconv.Itoa(node)

	req, err := http.NewRequest(http.MethodGet, panel+"api/application/nodes/"+n+"/allocations", nil)
	req.Header.Set("Content-type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return []byte{}, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}
	return body, nil
}
