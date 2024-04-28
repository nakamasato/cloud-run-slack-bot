package visualize

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/snapshot-chromedp/render"
	"github.com/nakamasato/cloud-run-slack-bot/pkg/monitoring"
)

// https://github.com/go-echarts/snapshot-chromedp/blob/47575f6f0d3957501fff8cd8fa89c6f3e97916a4/render/chromedp.go#L15-L21
const (
	HTML               = "html"
	FileProtocol       = "file://"
	EchartsInstanceDom = "div[_echarts_instance_]"
	CanvasJs           = "echarts.getInstanceByDom(document.querySelector('div[_echarts_instance_]'))" +
		".getDataURL({type: '%s', pixelRatio: %d, excludeComponents: ['toolbox']})"
)

// generate random data
func generateRandomData() *monitoring.TimeSeries {
	items := monitoring.TimeSeries{}
	for i := 0; i < 7; i++ {
		items = append(items, int64(rand.Intn(300))) // opts.LineData{Value: rand.Intn(300)}
	}
	return &items
}

var (
	weeks = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
)

func drawLineChart(title, subtitle string, xAxis *[]string, data *monitoring.TimeSeriesMap) *charts.Line {
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			BackgroundColor: "#FFFFFF",
		}),
		// Don't forget disable the Animation
		charts.WithAnimation(false),
		charts.WithTitleOpts(opts.Title{
			Title:    title,
			Subtitle: subtitle,
			Right:    "40%",
		}),
		charts.WithLegendOpts(opts.Legend{Right: "80%"}),
	)
	if xAxis != nil {
		line.SetXAxis(xAxis)
	}
	for name, items := range *data {
		data := getLineData(&items)
		log.Printf("name: %s, data: %v\n", name, data)
		line.AddSeries(name, *data)
	}
	return line
}

// Visualize draw a line chart and export to a file.
func Visualize(title, subtitle, fileName string, xAxis *[]string, data *monitoring.TimeSeriesMap) error {
	line := drawLineChart(title, subtitle, xAxis, data)
	return render.MakeChartSnapshot(line.RenderContent(), fileName)
}

func VisualizeSample(fileName string) error {
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithInitializationOpts(opts.Initialization{
			BackgroundColor: "#FFFFFF",
		}),
		// Don't forget disable the Animation
		charts.WithAnimation(false),
		charts.WithTitleOpts(opts.Title{
			Title:    "Line-Chart",
			Subtitle: "Example Subtitle",
			Right:    "40%",
		}),
		charts.WithLegendOpts(opts.Legend{Right: "80%"}),
	)
	line.SetXAxis(weeks).
		AddSeries("A", *getLineData(generateRandomData())).
		AddSeries("B", *getLineData(generateRandomData())).
		AddSeries("C", *getLineData(generateRandomData())).
		AddSeries("D", *getLineData(generateRandomData()))
	return MakeChartSnapshot(line.RenderContent(), fileName)
}

func getLineData(data *monitoring.TimeSeries) *[]opts.LineData {
	items := make([]opts.LineData, 0)
	for i, v := range *data {
		items = append(items, opts.LineData{Value: v, XAxisIndex: i})
	}
	return &items
}

// https://github.com/go-echarts/snapshot-chromedp/blob/47575f6f0d3957501fff8cd8fa89c6f3e97916a4/render/chromedp.go#L42-L56
func MakeChartSnapshot(content []byte, image string) error {
	path, file := filepath.Split(image)
	suffix := filepath.Ext(file)[1:]
	fileName := file[0 : len(file)-len(suffix)-1]

	config := &render.SnapshotConfig{
		RenderContent: content,
		Path:          path,
		FileName:      fileName,
		Suffix:        suffix,
		Quality:       1,
		KeepHtml:      false,
	}
	return MakeSnapshot(config)
}

func MakeSnapshot(config *render.SnapshotConfig) error {
	path := config.Path
	fileName := config.FileName
	content := config.RenderContent
	quality := config.Quality
	suffix := config.Suffix
	keepHtml := config.KeepHtml
	htmlPath := config.HtmlPath

	if htmlPath == "" {
		htmlPath = path
	}

	if !filepath.IsAbs(path) {
		path, _ = filepath.Abs(path)
	}

	if !filepath.IsAbs(htmlPath) {
		htmlPath, _ = filepath.Abs(htmlPath)
	}

	var ctx context.Context
	var cancel context.CancelFunc
	if chromedpRemoteUrl := os.Getenv("CHROMEDP_REMOTE_URL"); chromedpRemoteUrl == "" { // "wss://localhost:9222"
		ctx = context.Background()
	} else {
		ctx, cancel = chromedp.NewRemoteAllocator(context.Background(), chromedpRemoteUrl)
		defer cancel()
	}

	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()

	htmlFullPath := filepath.Join(htmlPath, fileName+"."+HTML)

	if !keepHtml {
		defer func() {
			err := os.Remove(htmlFullPath)
			if err != nil {
				log.Printf("Failed to delete the file(%s), err: %s\n", htmlFullPath, err)
			}
		}()
	}

	err := os.WriteFile(htmlFullPath, content, 0o644)
	if err != nil {
		return err
	}

	if quality < 1 {
		quality = 1
	}

	var base64Data string
	executeJS := fmt.Sprintf(CanvasJs, suffix, quality)
	err = chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("%s%s", FileProtocol, htmlFullPath)),
		chromedp.WaitVisible(EchartsInstanceDom, chromedp.ByQuery),
		chromedp.Evaluate(executeJS, &base64Data),
	)
	if err != nil {
		return err
	}

	imgContent, err := base64.StdEncoding.DecodeString(strings.Split(base64Data, ",")[1])
	if err != nil {
		return err
	}

	imageFullPath := filepath.Join(path, fmt.Sprintf("%s.%s", fileName, suffix))
	if err := os.WriteFile(imageFullPath, imgContent, 0o644); err != nil {
		return err
	}

	log.Printf("Wrote %s.%s success", fileName, suffix)
	return nil
}
