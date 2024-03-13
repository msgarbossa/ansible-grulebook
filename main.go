package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/hyperjumptech/grule-rule-engine/ast"
	"github.com/hyperjumptech/grule-rule-engine/builder"
	"github.com/hyperjumptech/grule-rule-engine/engine"
	"github.com/hyperjumptech/grule-rule-engine/pkg"

	"gopkg.in/yaml.v3"
)

// global variables
var (
	BuildVersion   string = ""
	BuildDate      string = ""
	httpAddr       string = ":5000"
	regExpHostPort        = regexp.MustCompile(`(^[a-zA-Z0-9./\-_]+)\:([0-9]{2,5})$`)
)

type AlertManagerPost struct {
	Receiver string `json:"receiver"`
	Status   string `json:"status"`
	Alerts   []struct {
		Alert
	} `json:"alerts"`
	ExternalURL     string `json:"externalURL"`
	Version         string `json:"version"`
	GroupKey        string `json:"groupKey"`
	TruncatedAlerts int    `json:"truncatedAlerts"`
}

type Alert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations struct {
		Description string `json:"description"`
		Summary     string `json:"summary"`
	} `json:"annotations"`
	StartsAt      time.Time `json:"startsAt"`
	EndsAt        time.Time `json:"endsAt"`
	GeneratorURL  string    `json:"generatorURL"`
	Fingerprint   string    `json:"fingerprint"`
	Playbook      string
	InventoryFile string
	LimitHost     string
}

type Source struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type Config struct {
	Sources []Source `yaml:"sources"`
}

func (Fact *Alert) GetWhatToSay(sentence string) string {
	return fmt.Sprintf("Let say \"%s\"", sentence)
}

func (Fact *Alert) GetLimitHost() string {
	instance, ok := Fact.Labels["instance"]
	if !ok {
		return ""
	}
	if regExpHostPort.MatchString(instance) {
		instance = regExpHostPort.ReplaceAllString(instance, "${1}")
	} else {
		slog.Warn("could not match host/port in alert:" + instance)
		return ""
	}
	return instance
}

var (
	knowledgeBase *ast.KnowledgeBase
)

func readConf(filename string) (*Config, error) {
	buf, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	c := &Config{}
	err = yaml.Unmarshal(buf, c)
	if err != nil {
		return nil, fmt.Errorf("in file %q: %w", filename, err)
	}

	return c, err
}

func main() {

	// default log level to LevelWarn (prints WARN and ERROR)
	slog.SetLogLoggerLevel(slog.LevelInfo)

	var (
		//showVersion = fs.Bool("version", false, "Print the version")
		ctx, cancel = context.WithCancel(context.Background())
	)

	defer func() {
		fmt.Println("Running cancel()")
		cancel()
	}()

	config_obj, err := readConf("config.yml")
	if err != nil {
		panic(err)
	}

	// TODO: this should unmarshal JSON instead of looping
	for _, source_obj := range config_obj.Sources {
		httpAddr = fmt.Sprintf(":%d", source_obj.Port)
	}

	knowledgeBase = getRules()

	go startHttpListener(ctx)

	// Wait for SIGINT.
	sig := make(chan os.Signal, 3)
	signal.Notify(sig, syscall.SIGHUP)
	signal.Notify(sig, syscall.SIGINT)
	signal.Notify(sig, syscall.SIGTERM)
	<-sig

}

func getRules() *ast.KnowledgeBase {

	knowledgeLibrary := ast.NewKnowledgeLibrary()
	ruleBuilder := builder.NewRuleBuilder(knowledgeLibrary)

	filename := "rules.grl"
	rulefileBytes, err := os.ReadFile(filename)
	if err != nil {
		slog.Error("could not read rule file " + filename)
		//return err
	}

	drls := string(rulefileBytes)

	// Add the rule definition above into the library and name it 'TutorialRules'  version '0.0.1'
	bs := pkg.NewBytesResource([]byte(drls))
	err = ruleBuilder.BuildRuleFromResource("TutorialRules", "0.0.1", bs)
	if err != nil {
		panic(err)
	}

	knowledgeBase, err := knowledgeLibrary.NewKnowledgeBaseInstance("TutorialRules", "0.0.1")
	if err != nil {
		panic(err)
	}

	return knowledgeBase

}

func startHttpListener(ctx context.Context) {

	defer func() {
		log.Println("running deferred ctx.Done in startHttpListener")
		select {
		case <-ctx.Done():
			log.Println("startHttpListener cancelled due to error: ", ctx.Err())
			return
		default:
			log.Println("startHttpListener was not cancelled")
			return
		}
	}()

	http.HandleFunc("/alerts", handleWebhook)
	slog.Info("server started: " + httpAddr)
	log.Fatal(http.ListenAndServe(httpAddr, nil))

}

func (a *Alert) evaluateFactAgainstRules() {

	dataCtx := ast.NewDataContext()
	err := dataCtx.Add("Fact", a)
	if err != nil {
		panic(err)
	}

	// Create a new engine for this payload (MaxCycle sets maximum rules to evaluate)
	engine := &engine.GruleEngine{MaxCycle: 100}

	// Execute the KnowledgeBase against DataContext
	err = engine.Execute(dataCtx, knowledgeBase)
	if err != nil {
		panic(err)
	}

	// Log Playbook value set by rules
	slog.Info(a.Playbook)
	slog.Info(a.InventoryFile)
	slog.Info(a.LimitHost)

}

func processAlerts(bodyBytes *[]byte) {

	var amp AlertManagerPost
	if err := json.Unmarshal(*bodyBytes, &amp); err != nil {
		log.Fatal(err)
	}

	for _, AlertItem := range amp.Alerts {
		alertName, ok := AlertItem.Labels["alertname"]
		if !ok {
			log.Fatal("expected to find 'alertname'")
		}
		slog.Info(fmt.Sprintf("%s, %s, %s\n", alertName, AlertItem.Status, AlertItem.Labels))

		AlertItem.evaluateFactAgainstRules()

	}

}

func handleWebhook(w http.ResponseWriter, r *http.Request) {

	slog.Info(fmt.Sprintf("headers: %v\n", r.Header))

	bodyBytes, err := readBodyBytes((r))
	if err != nil {
		log.Println(err)
		return
	}

	slog.Debug(fmt.Sprintf("body: %s\n", bodyBytes))
	processAlerts(&bodyBytes)

	slog.Info("Finished processing webhook")

}

func readBodyBytes(r *http.Request) ([]byte, error) {
	// Read body
	bodyBytes, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		return nil, readErr
	}
	defer r.Body.Close()

	// GZIP decode
	if len(r.Header["Content-Encoding"]) > 0 && r.Header["Content-Encoding"][0] == "gzip" {
		contents, gzErr := gzip.NewReader(io.NopCloser(bytes.NewBuffer(bodyBytes)))
		if gzErr != nil {
			return nil, gzErr
		}
		defer contents.Close()

		bb, err2 := io.ReadAll(contents)
		if err2 != nil {
			return nil, err2
		}
		return bb, nil
	} else {
		// Not compressed
		return bodyBytes, nil
	}
}
