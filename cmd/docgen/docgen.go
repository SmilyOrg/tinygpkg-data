package main

import (
	"compress/gzip"
	_ "embed"
	"flag"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type CompressOpts struct {
	Name              string
	Simplify          []float64
	SimplifyMinPoints int
	MinPrecXY         int
}

type RenderOpts struct {
	Places []Place
}

type Place struct {
	Name string
}

type Readme struct {
	Parameters  []CompressOpts
	LargePlaces []string
	SmallPlaces []string
	Datasets    []Dataset
}

type Dataset struct {
	Name         string
	Variants     []Variant
	PreviewNames []string
	Render       RenderOpts
}

type Variant struct {
	Name     string
	Fullname string
	Path     string
	Size     string
	Previews []Preview
}

type Preview struct {
	Name string
	Path string
}

type Featured struct {
	Name   string
	Source string
	Desc   string
}

type WriteCounter struct {
	Total int64
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Total += int64(n)
	return n, nil
}

func main() {

	var templatePath string
	var rootDir string
	var dataDir string
	var dataUri string
	var output string

	flag.StringVar(&templatePath, "template", "", "path to the template file")
	flag.StringVar(&rootDir, "root", "./", "root directory")
	flag.StringVar(&dataDir, "data", rootDir+"data/", "data directory")
	flag.StringVar(&dataUri, "download", rootDir+"data/", "download uri prefix")
	flag.StringVar(&output, "output", rootDir+"README.md", "output file")
	flag.Parse()

	if templatePath == "" {
		panic("template path not set")
	}

	templateStr, err := os.ReadFile(templatePath)
	if err != nil {
		panic(err)
	}

	gzipCache := map[string]int64{}

	t, err := template.
		New("readme").
		Funcs(template.FuncMap{
			"filesize": func(path string) int64 {
				stat, err := os.Stat(rootDir + path)
				if err != nil {
					return 0
					// return "❓"
				}
				return stat.Size()
			},
			"gzipfilesize": func(path string) int64 {
				if size, ok := gzipCache[path]; ok {
					return size
				}

				f, err := os.Open(rootDir + path)
				if err != nil {
					return 0
				}
				defer f.Close()

				wc := &WriteCounter{}
				gw := gzip.NewWriter(wc)
				if _, err = io.Copy(gw, f); err != nil {
					return 0
				}
				gw.Close()
				gzipCache[path] = wc.Total
				return wc.Total
			},
			"kb": func(b int64) string {
				return fmt.Sprintf("%d\u202FKB", b/1000)
			},
			"variants": func(name, glob string, previews []string, suffix string) []Variant {
				return getVariants(name, dataDir+name+glob+".gpkg", previews, suffix)
			},
			"gpkg": func(name string) string {
				return name + ".gpkg"
			},
			"download": func(name string) string {
				return dataUri + name
			},
			"local": func(name string) string {
				return dataDir + name
			},
			"percent": func(a, b int64) string {
				frac := float64(a) / float64(b)
				return fmt.Sprintf("%.1f%%", frac*100)
			},
			"featured": func(name, source, desc string) Featured {
				return Featured{
					Name:   name,
					Source: source,
					Desc:   desc,
				}
			},
		}).
		Parse(string(templateStr))

	if err != nil {
		panic(err)
	}

	f, err := os.Create(output)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	f.WriteString("<!-- Generated from " + templatePath + " DO NOT EDIT -->\n\n")

	readme := Readme{
		LargePlaces: []string{
			"world",
			"europe",
			"africa",
			"usa",
			"japan",
		},
		SmallPlaces: []string{
			"world",
			"berlin",
			"nyc",
			"tokyo",
			"ljubljana",
		},
	}

	err = t.Execute(f, readme)
	if err != nil {
		panic(err)
	}
}

func getVariants(name, glob string, previewNames []string, previewSuffix string) []Variant {
	files, err := filepath.Glob(glob)
	if err != nil {
		panic(err)
	}
	var variants []Variant
	for _, file := range files {
		base := filepath.Base(file)
		noext := strings.TrimSuffix(base, filepath.Ext(file))
		suffix := strings.TrimPrefix(noext, name+"_")
		dir := filepath.Dir(file)
		pbase := name + "_" + previewSuffix + suffix
		previews := []Preview{}
		for _, place := range previewNames {
			pfile := filepath.Join(dir, pbase+"_"+place+".png")
			if _, err := os.Stat(pfile); os.IsNotExist(err) {
				continue
			}
			previews = append(previews, Preview{
				Name: place,
				Path: filepath.ToSlash(pfile),
			})
		}
		variant := Variant{
			Name:     suffix,
			Fullname: noext,
			Previews: previews,
		}
		variants = append(variants, variant)
	}
	return variants
}
