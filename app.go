package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	fthealth "github.com/Financial-Times/go-fthealth/v1a"
	status "github.com/Financial-Times/service-status-go/httphandlers"
	"github.com/gorilla/mux"
	"github.com/jawher/mow.cli"
)

var (
	urlList = [][]string{
		{"lists", "/__document-store-api/lists/f91b1e6a-5e21-11e6-a72a-bd4bf1198c63"},
		{"content", "/__document-store-api/content/bd1cecf2-893e-11e6-8cb7-e7ada1d123b1"},
	}
)

func main() {
	app := cli.App("public-api-checker", "A checker for business level API endpoints")
	port := app.Int(cli.IntOpt{
		Name:   "port",
		Value:  8080,
		Desc:   "Port to listen on",
		EnvVar: "APP_PORT",
	})
	baseURL := app.String(cli.StringOpt{
		Name:   "baseurl",
		Value:  "http://localhost:1234/",
		Desc:   "base URL for outgoing check requests (e.g., vulcand base URL)",
		EnvVar: "BASE_URL",
	})
	user := app.String(cli.StringOpt{
		Name:   "user",
		Value:  "",
		Desc:   "User for basic auth in outgoing check requests",
		EnvVar: "USER",
	})
	password := app.String(cli.StringOpt{
		Name:   "password",
		Value:  "",
		Desc:   "Password for basic auth in outgoing check requests",
		EnvVar: "PASSWORD",
	})

	app.Action = func() {
		runServer(*baseURL, *port, *user, *password)
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func runServer(baseURL string, port int, user, password string) {
	log.Printf("starting on port %d\n", port)

	servicesRouter := mux.NewRouter()

	hcs := makeHealthChecks(baseURL, user, password)

	servicesRouter.HandleFunc("/__health", fthealth.Handler("Public API Checker healthchecks",
		"Checks for accessing public API endpoints", hcs...))

	http.HandleFunc(status.PingPath, status.PingHandler)
	http.HandleFunc(status.PingPathDW, status.PingHandler)
	http.HandleFunc(status.BuildInfoPath, status.BuildInfoHandler)
	http.HandleFunc(status.BuildInfoPathDW, status.BuildInfoHandler)
	g := &gtg{hcs}
	http.HandleFunc("/__gtg", g.serve)

	http.Handle("/", servicesRouter)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatalf("Unable to start server: %v", err)
	}
}

func makeHealthChecks(baseURL string, user, password string) (checks []fthealth.Check) {

	for _, entry := range urlList {
		id := entry[0]
		url := entry[1]

		fullURL := fmt.Sprintf("%s%s", baseURL, url)

		checks = append(checks, fthealth.Check{
			BusinessImpact:   fmt.Sprintf("%s appears to be failing at url %s", id, url),
			Name:             fmt.Sprintf("Check for url %s", url),
			PanicGuide:       fmt.Sprintf("Inspect the %s services in this cluster to find the problem(s)", id),
			Severity:         1, // This represents failure of a business function, so severity 1 it is.
			TechnicalSummary: "See specific service in question for more technical detail",
			Checker:          func() (string, error) { return checkHTTPOK(fullURL, user, password) },
		})
	}

	return
}

type gtg struct {
	hc []fthealth.Check
}

func (g *gtg) serve(w http.ResponseWriter, r *http.Request) {
	result := fthealth.RunCheck("unused", "ununsed", true, g.hc...)

	if result.Ok {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func checkHTTPOK(urlString, user, password string) (string, error) {

	req, err := http.NewRequest("GET", urlString, nil)
	if err != nil {
		return err.Error(), err
	}

	if user != "" {
		req.SetBasicAuth(user, password)
	}

	resp, err := http.DefaultClient.Do(req)

	defer func() {
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusOK {
		return "", nil
	}

	err = fmt.Errorf("check failed with status %d for url %s", resp.StatusCode, urlString)
	return err.Error(), err
}
