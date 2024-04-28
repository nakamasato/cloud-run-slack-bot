package visualize

import (
	"io"

	"github.com/wcharczuk/go-chart/v2"
)

// Visualize draw a line chart and export to a file.
func Visualize(writer io.Writer) error {
	graph := chart.Chart{
		Series: []chart.Series{
			chart.ContinuousSeries{
				XValues: []float64{1.0, 2.0, 3.0, 4.0},
				YValues: []float64{1.0, 2.0, 3.0, 4.0},
			},
		},
	}

	// buffer := bytes.NewBuffer([]byte{})
	return graph.Render(chart.PNG, writer)
}

func VisualizeSample(writer io.Writer) error {
	graph := chart.Chart{
		Series: []chart.Series{
			chart.ContinuousSeries{
				XValues: []float64{1.0, 2.0, 3.0, 4.0},
				YValues: []float64{1.0, 2.0, 3.0, 4.0},
			},
		},
	}

	// buffer := bytes.NewBuffer([]byte{})
	return graph.Render(chart.PNG, writer)
}
