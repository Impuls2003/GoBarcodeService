package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/datamatrix"
	"github.com/boombuler/barcode/ean"
	"github.com/boombuler/barcode/qr"

	"github.com/kardianos/service"
)

type Route struct {
	Path        string
	Handler     http.HandlerFunc
	Description string
	Example     string
}

type RequestParams struct {
	Text    string
	Width   int
	Height  int
	QrLevel qr.ErrorCorrectionLevel
}

// Config описывает настройки сервиса
type Config struct {
	Host string `json:"host"`
	Port string `json:"port"`
}

type program struct {
	config Config
}

// список маршрутов
var routes = []Route{
	{
		Path:        "/qr",
		Handler:     handleQR,
		Description: "QR-код",
		Example:     "/qr?text=Hello",
	},
	{
		Path:        "/datamatrix",
		Handler:     handleDataMatrix,
		Description: "DataMatrix",
		Example:     "/datamatrix?text=Hello",
	},
	{
		Path:        "/ean128",
		Handler:     handleEAN128,
		Description: "EAN128",
		Example:     "/ean128?text=Hello",
	},
	{
		Path:        "/ean13",
		Handler:     handleEAN13,
		Description: "EAN-13 (12 цифр, 13-я опционаяльная контрольная (вычисляется по необходимости))",
		Example:     "/ean13?text=123456789012",
	},
}

func (p *program) Start(s service.Service) error {
	// Запускаем сервер в отдельной горутине
	go p.run()
	return nil
}

func (p *program) run() {

	// Загружаем конфигурационный файл
	p.config = loadConfig()

	// Главная страница с документацией генерируется автоматически
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<h1>1D/2D code Generator Service</h1>
			<p>Доступные эндпоинты:</p>
			<ul>`)

		for _, route := range routes {
			fmt.Fprintf(w, `<li><b>%s</b> — %s<br>
				Пример: <a href="%s">%s</a></li>`,
				route.Path, route.Description, route.Example, route.Example)
		}

		fmt.Fprint(w, `</ul>
			<p>Параметры:</p>
			<ul>
				<li><b>text</b> — содержимое кода (обязательно)</li>
				<li><b>level</b> — уровень коррекции ошибок от 1 до 4 (опционально, по умолчанию 2)</li>
				<li><b>width</b> — ширина изображения в пикселях (опционально, по умолчанию 256)</li>
				<li><b>height</b> — высота изображения в пикселях (опционально, по умолчанию 128). Для Qr и DataMatrix - не учитывается</li>
			</ul>`)
	})

	// регистрация маршрутов
	for _, route := range routes {
		http.HandleFunc(route.Path, route.Handler)
	}

	addr := p.config.Host + ":" + p.config.Port
	log.Println("GoBarcodeService service started on " + addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func (p *program) Stop(s service.Service) error {
	log.Println("GoBarcodeService service stopped")
	return nil
}

func main() {
	svcConfig := &service.Config{
		Name:        "GoBarcodeService",
		DisplayName: "1D/2D code Generator Service",
		Description: "Сервис генерации 1D/2D кодов",
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}

	logger, err := s.Logger(nil)
	if err == nil {
		logger.Info("Starting GoBarcodeService")
	}

	if len(os.Args) > 1 {
		// Управление службой из консоли
		err = service.Control(s, os.Args[1])
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	err = s.Run()
	if err != nil {
		logger.Error(err)
	}
}

// Загружаем конфигурацию
func loadConfig() Config {

	// Определяем путь к установочному файлу
	exePath, err := os.Executable()
	if err != nil {
		log.Println("Unable to determine path to exe:", err)
		return Config{Host: "0.0.0.0", Port: "8080"}
	}

	configPath := filepath.Join(filepath.Dir(exePath), "config.json")

	// Ищем config.json
	file, err := os.Open(configPath)
	if err != nil {
		log.Println("config.json file not found, using port 0.0.0.0:8080")
		return Config{Host: "0.0.0.0", Port: "8080"}
	}
	defer file.Close()

	// Читаем config.json
	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		log.Println("Error reading config.json:", err)
		return Config{Host: "0.0.0.0", Port: "8080"}
	}

	if cfg.Host == "" {
		cfg.Host = "0.0.0.0"
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	return cfg
}

// Реализация функционала
// Добавить надпись на изображение
func addLabel(img image.Image, text string) image.Image {
	bounds := img.Bounds()
	// добавим место снизу под текст
	newHeight := bounds.Dy() + 20
	newImg := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), newHeight))

	// Залить белым фоном
	draw.Draw(newImg, newImg.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)

	// скопируем штрихкод
	draw.Draw(newImg, bounds, img, bounds.Min, draw.Src)

	// рисуем текст (черный цвет)
	col := color.RGBA{0, 0, 0, 255}
	d := &font.Drawer{
		Dst:  newImg,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(bounds.Dx()/2-len(text)*3, bounds.Dy()+15),
	}
	// вычисляем ширину текста в фиксированных координатах
	textWidth := d.MeasureString(text).Ceil()

	// центрируем по ширине картинки
	x := (bounds.Dx() - textWidth) / 2
	y := bounds.Dy() + 15

	d.Dot = fixed.P(x, y)
	d.DrawString(text)

	return newImg
}

// Парсим параметры, переданные в запросе.
func parseParams(r *http.Request) (*RequestParams, error) {

	if err := r.ParseForm(); err != nil {
		return nil, err
	}

	// Текст
	text := r.Form.Get("text")
	if text == "" {
		return nil, fmt.Errorf("missing text")
	}

	// Размер
	width := 256
	if wStr := r.Form.Get("width"); wStr != "" {
		if wInt, err := strconv.Atoi(wStr); err == nil && wInt > 0 {
			width = wInt
		}
	}

	height := width / 2
	if hStr := r.Form.Get("height"); hStr != "" {
		if hInt, err := strconv.Atoi(hStr); err == nil && hInt > 0 {
			height = hInt
		}
	}

	// Коррекция qr
	qrLevel := qr.M
	if cStr := r.URL.Query().Get("level"); cStr != "" {
		if cInt, err := strconv.Atoi(cStr); err == nil {
			switch cInt {
			case 1:
				qrLevel = qr.L
			case 2:
				qrLevel = qr.M
			case 3:
				qrLevel = qr.Q
			case 4:
				qrLevel = qr.H
			default:
				qrLevel = qr.M
			}
		}
	}

	return &RequestParams{
		Text:    text,
		Width:   width,
		Height:  height,
		QrLevel: qrLevel,
	}, nil
}

func handleQR(w http.ResponseWriter, r *http.Request) {

	// Получаем параметры для генерации кода
	params, err := parseParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	code, err := qr.Encode(params.Text, params.QrLevel, qr.Auto)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	code, err = barcode.Scale(code, params.Width, params.Width)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")

	// Кодируем в ЧБ палитру. Это уменьшит размер передаваемых данных. Вообще можно было бы просто сделать png.Encode(w, code)
	bwImg := image.NewPaletted(code.Bounds(), color.Palette{color.White, color.Black})
	draw.FloydSteinberg.Draw(bwImg, code.Bounds(), code, image.Point{})
	png.Encode(w, bwImg)
}

func handleDataMatrix(w http.ResponseWriter, r *http.Request) {
	// Получаем параметры для генерации кода
	params, err := parseParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	code, err := datamatrix.Encode(params.Text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	code, err = barcode.Scale(code, params.Width, params.Width)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")

	// Кодируем в ЧБ палитру. Это уменьшит размер передаваемых данных. Вообще можно было бы просто сделать png.Encode(w, code)
	bwImg := image.NewPaletted(code.Bounds(), color.Palette{color.White, color.Black})
	draw.FloydSteinberg.Draw(bwImg, code.Bounds(), code, image.Point{})
	png.Encode(w, bwImg)
}

func handleEAN128(w http.ResponseWriter, r *http.Request) {

	// Получаем параметры для генерации кода
	params, err := parseParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	code, err := code128.Encode(params.Text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	scaled, err := barcode.Scale(code, params.Width, params.Height)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	withLabel := addLabel(scaled, params.Text)

	w.Header().Set("Content-Type", "image/png")
	//w.Header().Set("Content-Type", "application/octet-stream")

	// Кодируем в ЧБ палитру. Это уменьшит размер передаваемых данных. Вообще можно было бы просто сделать png.Encode(w, scaled)
	// bwImg := image.NewPaletted(scaled.Bounds(), color.Palette{color.White, color.Black})
	// draw.FloydSteinberg.Draw(bwImg, scaled.Bounds(), code, image.Point{})
	png.Encode(w, withLabel)
}

func handleEAN13(w http.ResponseWriter, r *http.Request) {

	// Получаем параметры для генерации кода
	params, err := parseParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	code, err := ean.Encode(params.Text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	scaled, err := barcode.Scale(code, params.Width, params.Height)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	withLabel := addLabel(scaled, params.Text)

	w.Header().Set("Content-Type", "image/png")
	//w.Header().Set("Content-Type", "application/octet-stream")

	// Кодируем в ЧБ палитру. Это уменьшит размер передаваемых данных. Вообще можно было бы просто сделать png.Encode(w, scaled)
	// bwImg := image.NewPaletted(scaled.Bounds(), color.Palette{color.White, color.Black})
	// draw.FloydSteinberg.Draw(bwImg, scaled.Bounds(), code, image.Point{})
	png.Encode(w, withLabel)
}
