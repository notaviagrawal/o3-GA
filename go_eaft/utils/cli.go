package utils

// This part is really important. CLI Interface will implement in here.
// Data will come from callback function. Maybe each run time could be here.

// GAUGE => Progress bar. This will include completed part of GA or other staff.
// Data : Total iteration, Hardness : Easy

// Line Chart => Each population best fitness value. This shows general of ga.
// Data : Hall of Fame, Hardness : Easy

// Sparkline :> Every single run time value. Different from  line chart because this will show everything.
// Data :> Compile Code return value : Total. Hardness : Hard

// List -> List of Specifications. For example : mutation rate, crossover rate, model etc.
// Text Box -> Not sure about it but it could be name of session.

import (
	"log"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

var TotalRunTimes []float64
var Notifications []string
var TextBox string
var Progress float64
var HallOfFame float64
var Stats []float64
var BestOfPops []float64

func CLI() {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	p := widgets.NewParagraph()
	p.Title = "Callback Notification"
	p.Text = TextBox
	p.SetRect(0, 0, 50, 5)
	p.TextStyle.Fg = ui.ColorWhite
	p.BorderStyle.Fg = ui.ColorWhite

	updateParagraph := func(count int) {
		p.Text = TextBox
	}

	l := widgets.NewList()
	l.Title = "List"
	l.Rows = Notifications
	l.SetRect(0, 5, 25, 12)
	l.TextStyle.Fg = ui.ColorYellow

	g := widgets.NewGauge()
	g.Title = "Progress"
	g.Percent = 50
	g.SetRect(0, 12, 50, 15)
	g.BarColor = ui.ColorYellow
	g.BorderStyle.Fg = ui.ColorWhite
	g.TitleStyle.Fg = ui.ColorWhite

	sl := widgets.NewSparkline()
	sl.Title = "srv 0:"
	sl.Data = make([]float64, 0)
	sl.LineColor = ui.ColorCyan
	sl.TitleStyle.Fg = ui.ColorWhite

	slg := widgets.NewSparklineGroup(sl)
	slg.Title = "Sparkline"
	slg.SetRect(25, 5, 50, 12)

	lc := widgets.NewPlot()
	lc.Title = "Hall of Fame"
	lc.Data = make([][]float64, 1)
	lc.Data[0] = append(lc.Data[0], 0.0)
	BestOfPops = append(BestOfPops, 0)
	lc.SetRect(0, 15, 50, 25)
	lc.AxesColor = ui.ColorWhite
	lc.LineColors[0] = ui.ColorRed
	lc.Marker = widgets.MarkerDot

	bc := widgets.NewBarChart()
	bc.Title = "Bar Chart"
	bc.SetRect(50, 0, 76, 10)
	bc.Labels = []string{"Min", "Max", "Avg"}
	bc.BarColors = []ui.Color{ui.ColorYellow, ui.ColorCyan}
	bc.LabelStyles = []ui.Style{ui.NewStyle(ui.ColorWhite)}
	bc.NumStyles = []ui.Style{ui.NewStyle(ui.ColorBlack)}
	bc.BarWidth = 5
	bc.BarGap = 4

	lc2 := widgets.NewPlot()
	lc2.Title = "Current Fitness Values"
	lc2.Data = make([][]float64, 1)
	lc2.Data[0] = append(lc2.Data[0], 0.0)
	TotalRunTimes = append(TotalRunTimes, 0, 0, 0)
	lc2.SetRect(50, 15, 76, 25)
	lc2.AxesColor = ui.ColorWhite
	lc2.LineColors[0] = ui.ColorYellow

	draw := func(count int) {
		if g.Percent >= 100 {
			log.Println("Best Flag Set Saved. It was good to see you ...")
			ui.Close()
		}

		g.Percent = int(Progress)
		if len(Notifications) > 0 {
			l.Rows = Notifications
		}
		slg.Sparklines[0].Data = TotalRunTimes
		lc.Data[0] = BestOfPops
		if len(TotalRunTimes) > 0 {
			if len(TotalRunTimes) > 16 {
				lc2.Data[0] = TotalRunTimes[len(TotalRunTimes)-15:]
				slg.Sparklines[0].Data = TotalRunTimes[len(TotalRunTimes)-15:]
			} else {
				lc2.Data[0] = TotalRunTimes
			}
		}
		bc.Data = Stats
		ui.Render(p, l, g, slg, lc, bc, lc2)
	}

	tickerCount := 1
	draw(tickerCount)
	tickerCount++
	uiEvents := ui.PollEvents()
	ticker := time.NewTicker(time.Second).C
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			}
		case <-ticker:
			updateParagraph(tickerCount)
			draw(tickerCount)
			tickerCount++
		}
	}
}
