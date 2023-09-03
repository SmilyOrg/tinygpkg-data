package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/bitfield/script"
	"github.com/golang/geo/s2"
	"github.com/peterstace/simplefeatures/geom"
	"github.com/smilyorg/tinygpkg/gpkg"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func Build() error {
	println("hello world")
	return nil
}

//go:embed README.tmpl.md
var readmeTemplate string

const OUTPUT_DIR = "./"
const DATA_DIR = OUTPUT_DIR + "data/"
const NATURAL_EARTH_URL = "https://raw.githubusercontent.com/nvkelso/natural-earth-vector/117488dc884bad03366ff727eca013e434615127/geojson/"
const GEOBOUNDARIES_URL = "https://github.com/wmgeolab/geoBoundaries/raw/742b9ae7d6a57e57fe6d47f29343bc1081a8f09d/releaseData/"
const GDAL_DOCKER_IMAGE = "ghcr.io/osgeo/gdal:alpine-normal-3.7.1"
const MIN_PRECXY = -8
const MAX_PRECXY = +7

func ifNotExists(path string, fn func() error) error {
	_, err := os.Stat(path)
	if err == nil {
		fmt.Printf("exists %s\n", path)
		return nil
	}
	return fn()
}

func download(url string, path string) {
	err := ifNotExists(path, func() error {
		fmt.Printf("download %s\n", path)
		_, err := script.
			Get(url).
			WriteFile(path)
		fmt.Printf("download %s done\n", path)
		return err
	})
	if err != nil {
		panic(err)
	}
}

func gdal(mount string, cmd string) error {
	abs, err := filepath.Abs(mount)
	if err != nil {
		return fmt.Errorf("unable to get absolute path: %s", err.Error())
	}
	dcmd := fmt.Sprintf(
		`docker run --rm -v "%s:/data/" -w /data/ %s %s`,
		abs, GDAL_DOCKER_IMAGE, cmd,
	)
	fmt.Printf("gdal %s\n", cmd)
	script.Exec(dcmd).Stdout()
	// Ignore errors, often they don't mean anything
	return nil
}

func rasterize(srcPath string, table string, dstPath string, w, h int, xmin, ymin, xmax, ymax float64) error {
	sdir, sfile := filepath.Split(srcPath)
	ddir, dfile := filepath.Split(dstPath)
	if sdir != ddir {
		return fmt.Errorf("files need to share the same directory: %s %s", srcPath, dstPath)
	}

	// gdal_rasterize -init 255 -burn 90 -ot Byte -ts 2073 1000 -te 0 30 33 56 -l ne_110m_admin_0_countries ne_110m_admin_0_countries_roundtrip_twkb_p3_s1.gpkg output.tiff
	cmd := fmt.Sprintf(
		"gdal_rasterize -q -init 255 -burn 90 -ot Byte -ts %d %d -te %f %f %f %f -l %s %s %s",
		w, h, xmin, ymin, xmax, ymax, table, sfile, dfile,
	)
	return gdal(sdir, cmd)
}

func render(gpkgPath string, table string, pngPath string, w, h int, latlng s2.LatLng, zoom float64) error {
	dir, pngfile := filepath.Split(pngPath)
	name := strings.TrimSuffix(pngfile, filepath.Ext(pngfile))

	tifffile := name + ".tiff"
	tiff := dir + tifffile

	multisample := 4
	mw, mh := w*multisample, h*multisample

	// s :=

	ar := float64(h) / float64(w) * 360. / 180.

	p := math.Pow(2, zoom)

	cx, cy := latlng.Lng.Degrees(), latlng.Lat.Degrees()

	xs := 360 / p * 0.5
	xmin, xmax := cx-xs, cx+xs

	ys := 180 / p * 0.5
	ymin, ymax := cy-ys*ar, cy+ys*ar

	// don't check error code as gdal returns 61 for some reason,
	// but it works anyway ¯\_(ツ)_/¯
	rasterize(gpkgPath, table, tiff, mw, mh, xmin, ymin, xmax, ymax)
	if _, err := os.Stat(tiff); err != nil {
		return fmt.Errorf("tiff does not exist after rasterize: %s", err.Error())
	}

	// it's error code 89 here for another reason ¯\_(ツ)_/¯
	gdal(dir, fmt.Sprintf(`gdal_translate -q -of PNG -r lanczos -outsize %d %d %s %s`, w, h, tifffile, pngfile))
	if _, err := os.Stat(pngPath); err != nil {
		return fmt.Errorf("png does not exist after resize: %s", err.Error())
	}

	os.Remove(tiff)
	return nil
}

func convertToGeopackage(srcPath string, gpkgPath string) {
	err := ifNotExists(gpkgPath, func() error {
		sdir, sfile := filepath.Split(srcPath)
		gdir, gfile := filepath.Split(gpkgPath)
		if sdir != gdir {
			return fmt.Errorf("files need to share the same directory: %s %s", srcPath, gpkgPath)
		}

		abs, err := filepath.Abs(sdir)
		if err != nil {
			return err
		}
		return gdal(abs, fmt.Sprintf(`ogr2ogr -makevalid -f "GPKG" "%s" "%s"`, gfile, sfile))
	})
	if err != nil {
		panic(err)
	}
}

type CompressOpts struct {
	Name              string
	Simplify          []float64
	SimplifyMinPoints int
	MinPrecXY         int
}

type compressJob struct {
	fid int64
	wkb []byte
	h   gpkg.Header
}

type compressInfo struct {
	debug            string
	usedSimplify     float64
	usedPrecXY       int
	originalSize     int
	originalPoints   int
	simplifiedPoints int
}

type writeJob struct {
	name string
	fid  int64
	twkb []byte
	h    gpkg.Header
	info compressInfo
}

func compressor(wg *sync.WaitGroup, in <-chan compressJob, out chan<- writeJob, opts CompressOpts) {
	for job := range in {
		twkb, info, err := compressGeometry(job.wkb, opts)
		if err != nil {
			panic(err)
		}
		if twkb == nil {
			continue
		}
		out <- writeJob{opts.Name, job.fid, twkb, job.h, info}
	}
	wg.Done()
}

func writer(wg *sync.WaitGroup, writec *sqlite.Conn, table string, in <-chan writeJob) {
	writes := writec.Prep(fmt.Sprintf(`
		UPDATE %s
		SET geom = ?
		WHERE fid = ?`,
		table,
	))
	for job := range in {
		fid := job.fid
		g := job.h
		twkb := job.twkb
		info := job.info

		g.SetType(gpkg.ExtendedType)
		g.SetEnvelopeContentsIndicatorCode(gpkg.NoEnvelope)

		// Write TWKB
		w := new(bytes.Buffer)
		g.Write(w)
		w.Write([]byte{'T', 'W', 'K', 'B'})
		io.Copy(w, bytes.NewReader(twkb))

		buf := w.Bytes()
		writes.BindBytes(1, buf)
		writes.BindInt64(2, fid)
		_, err := writes.Step()
		if err != nil {
			panic(fmt.Errorf("unable to write twkb: %s", err.Error()))
		}
		err = writes.Reset()
		if err != nil {
			panic(fmt.Errorf("unable to reset write statement: %s", err.Error()))
		}

		fmt.Printf(
			"compress %10s fid %6d %.0e simplify %6d to %6d points %7d wkb bytes %7d twkb bytes %4.0f%% at %d precXY %6d bytes written %s\n",
			job.name, fid, info.usedSimplify, info.originalPoints, info.simplifiedPoints, info.originalSize, len(twkb), 100.*float32(len(twkb))/float32(info.originalSize), info.usedPrecXY, len(buf), info.debug,
		)
	}
	wg.Done()
}

func tempCopy(src string, dst string) (string, func(), error) {
	tmp := dst + ".tmp"
	_, err := script.Exec(fmt.Sprintf(`cp "%s" "%s"`, src, tmp)).Stdout()
	if err != nil {
		return tmp, nil, err
	}

	return tmp, func() {
		script.Exec(fmt.Sprintf(`mv "%s" "%s"`, tmp, dst)).Stdout()
	}, nil
}

func compressGeopackage(gpkgPath, tgpkgPath, table string, opts CompressOpts) error {
	// tmp := tgpkgPath + ".tmp"
	// _, err := script.Exec(fmt.Sprintf(`cp "%s" "%s"`, gpkgPath, tmp)).Stdout()
	tmp, move, err := tempCopy(gpkgPath, tgpkgPath)
	if err != nil {
		return err
	}
	defer move()
	pool, err := sqlitex.Open(tmp, 0, 2)
	if err != nil {
		return err
	}
	defer pool.Close()

	readc := pool.Get(context.Background())
	defer pool.Put(readc)

	reads := readc.Prep(fmt.Sprintf(`
		SELECT fid, geom
		FROM %s`,
		table,
	))
	defer reads.Reset()

	writec := pool.Get(context.Background())
	defer pool.Put(writec)

	err = sqlitex.ExecuteScript(writec, fmt.Sprintf(`
		INSERT OR IGNORE INTO gpkg_extensions VALUES ('%[1]s', 'geom', 'mlunar_twkb', 'https://github.com/SmilyOrg/tinygpkg', 'read-write');
		DROP TRIGGER IF EXISTS rtree_%[1]s_geom_update1;
		DROP TRIGGER IF EXISTS rtree_%[1]s_geom_update2;
		DROP TRIGGER IF EXISTS rtree_%[1]s_geom_update3;
		DROP TRIGGER IF EXISTS rtree_%[1]s_geom_update4;
	`, table), nil)
	if err != nil {
		return err
	}

	sqlitex.Execute(writec, "BEGIN TRANSACTION;", nil)

	compressChan := make(chan compressJob, 10)
	writeChan := make(chan writeJob, 10)
	cwg := &sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		cwg.Add(1)
		go compressor(cwg, compressChan, writeChan, opts)
	}
	wwg := &sync.WaitGroup{}
	wwg.Add(1)
	go writer(wwg, writec, table, writeChan)

	for {
		if exists, err := reads.Step(); err != nil {
			return fmt.Errorf("error listing geometry: %s", err.Error())
		} else if !exists {
			break
		}

		fid := reads.ColumnInt64(0)
		r := reads.ColumnReader(1)
		g, err := gpkg.Read(r)
		if err != nil {
			return fmt.Errorf("error reading gpkg: %s", err.Error())
		}

		if g.Empty() {
			fmt.Printf("compress fid %6d empty, skipping\n", fid)
			continue
		}

		if g.Type() != gpkg.StandardType {
			fmt.Printf("compress fid %6d non-standard geometry, skipping\n", fid)
			continue
		}

		wkb, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("error reading geometry: %s", err.Error())
		}

		compressChan <- compressJob{
			fid: fid,
			wkb: wkb,
			h:   *g,
		}

		// twkb, info, err := compressGeometry(wkb, opts)
		// if err != nil {
		// 	return fmt.Errorf("error compressing geometry: %s", err.Error())
		// }
	}

	close(compressChan)
	cwg.Wait()
	close(writeChan)
	wwg.Wait()

	err = sqlitex.Execute(writec, "COMMIT;", nil)
	if err != nil {
		return fmt.Errorf("unable to commit: %s", err.Error())
	}

	err = sqlitex.Execute(writec, "VACUUM;", nil)
	if err != nil {
		return fmt.Errorf("unable to vacuum: %s", err.Error())
	}
	return nil
}

func compressGeometry(wkb []byte, opts CompressOpts) (twkb []byte, info compressInfo, err error) {
	info.originalSize = len(wkb)

	gm, err := geom.UnmarshalWKB(wkb)
	if err != nil {
		return nil, info, fmt.Errorf("error unmarshalling wkb: %s", err.Error())
	}

	var extra []string

	info.originalPoints = gm.DumpCoordinates().Length()
	info.simplifiedPoints = info.originalPoints
	info.usedSimplify = 0.0
	if info.originalPoints < opts.SimplifyMinPoints {
		extra = append(extra, "not simplified min points")
	} else {
		si := 0
		for ; si < len(opts.Simplify); si++ {
			info.usedSimplify = opts.Simplify[si]
			gms, err := gm.Simplify(info.usedSimplify)
			if err == nil {
				points := gms.DumpCoordinates().Length()
				if points >= 3 {
					gm = gms
					info.simplifiedPoints = points
					break
				}
				extra = append(extra, "not simplified collapse")
				break
			}
		}
		if si == len(opts.Simplify) {
			extra = append(extra, "not simplified max level")
		} else if si > 0 {
			extra = append(extra, "simplify fallback")
		}
	}

	info.usedPrecXY = opts.MinPrecXY
	for ; info.usedPrecXY <= MAX_PRECXY; info.usedPrecXY++ {
		twkb, err = geom.MarshalTWKB(gm, info.usedPrecXY)
		if err != nil {
			return nil, info, fmt.Errorf("error marshalling twkb: %s", err.Error())
		}

		_, err = geom.UnmarshalTWKB(twkb)
		if err == nil {
			break
		}
		// if info.usedPrecXY >= MAX_PRECXY {
		// 	extra = append(extra, "unable to compress, skipping")
		// 	twkb = nil
		// 	// return nil, info, fmt.Errorf("error marshalling twkb: unable to roundtrip at any precision: %s", err.Error())
		// 	break
		// }
		// fmt.Printf("compress fid %6d roundtrip error, falling back to higher precision, precxy %d: %s\n", fid, prec, err.Error())
	}

	if info.usedPrecXY >= MAX_PRECXY {
		extra = append(extra, "unable to compress, skipping")
		twkb = nil
	} else if info.usedPrecXY != opts.MinPrecXY {
		extra = append(extra, "roundtrip fallback")
	}

	info.debug = strings.Join(extra, " ")
	return twkb, info, nil
}

func decompressGeopackage(name, tgpkgPath, gpkgPath, table string) error {
	// _, err := script.Exec(fmt.Sprintf(`cp "%s" "%s"`, tgpkgPath, gpkgPath)).Stdout()
	tmp, move, err := tempCopy(tgpkgPath, gpkgPath)
	if err != nil {
		return err
	}
	defer move()
	pool, err := sqlitex.Open(tmp, 0, 2)
	if err != nil {
		return err
	}
	defer pool.Close()

	readc := pool.Get(context.Background())
	defer pool.Put(readc)

	reads := readc.Prep(fmt.Sprintf(`
		SELECT fid, geom
		FROM %s`,
		table,
	))
	defer reads.Reset()

	writec := pool.Get(context.Background())
	defer pool.Put(writec)

	writes := writec.Prep(fmt.Sprintf(`
		UPDATE %s
		SET geom = ?
		WHERE fid = ?`,
		table,
	))

	sqlitex.Execute(writec, "BEGIN TRANSACTION;", nil)

	for {
		if exists, err := reads.Step(); err != nil {
			return fmt.Errorf("error listing geometry: %s", err.Error())
		} else if !exists {
			break
		}

		fid := reads.ColumnInt64(0)
		r := reads.ColumnReader(1)
		g, err := gpkg.Read(r)
		if err != nil {
			return fmt.Errorf("unable to read gpkg: %w", err)
		}

		if g.Empty() {
			fmt.Printf("decompress fid %6d empty, skipping\n", fid)
			continue
		}

		if g.Type() != gpkg.ExtendedType {
			fmt.Printf("decompress fid %6d standard geometry, skipping\n", fid)
			continue
		}

		var magic [4]byte
		n, err := r.Read(magic[:])
		if err != nil {
			return fmt.Errorf("unable to read magic: %w", err)
		}
		if n != 4 {
			return fmt.Errorf("unable to read magic, short read: %d", n)
		}
		if magic != [4]byte{'T', 'W', 'K', 'B'} {
			return fmt.Errorf("invalid magic: %s", string(magic[:]))
		}

		twkb, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("unable to read twkb: %w", err)
		}

		gm, err := geom.UnmarshalTWKB(twkb)
		if err != nil {
			return fmt.Errorf("unable to unmarshal twkb: %w", err)
		}

		wkb := gm.AsBinary()

		g.SetType(gpkg.StandardType)
		g.SetEnvelopeContentsIndicatorCode(gpkg.NoEnvelope)

		// Write WKB
		w := new(bytes.Buffer)
		g.Write(w)
		io.Copy(w, bytes.NewReader(wkb))

		buf := w.Bytes()
		writes.BindBytes(1, buf)
		writes.BindInt64(2, fid)
		_, err = writes.Step()
		if err != nil {
			return fmt.Errorf("unable to write wkb: %s", err.Error())
		}
		err = writes.Reset()
		if err != nil {
			return fmt.Errorf("unable to reset write statement: %w", err)
		}

		fmt.Printf(
			"decompress %10s fid %6d %7d twkb bytes to %7d wkb bytes %4.0f%% %7d bytes written\n",
			name, fid, len(twkb), len(wkb), 100.*float32(len(wkb))/float32(len(twkb)), len(buf),
		)
	}

	sqlitex.Execute(writec, "COMMIT;", nil)

	err = sqlitex.ExecuteScript(writec, fmt.Sprintf(`
		DELETE FROM gpkg_extensions WHERE table_name = '%[1]s' AND column_name = 'geom' AND extension_name = 'mlunar_twkb';
	`, table), nil)
	if err != nil {
		return err
	}

	err = sqlitex.Execute(writec, "VACUUM;", nil)
	if err != nil {
		return err
	}

	return nil
}

// func move(from string, to string) {
// 	_, err := script.Exec(fmt.Sprintf(`mv "%s" "%s"`, from, to)).Stdout()
// 	if err != nil {
// 		panic(err)
// 	}
// }

// func moveDir(path string, old string, new string) {
// 	after, found := strings.CutPrefix(path, old)
// 	if !found {
// 		panic("old dir not found")
// 	}
// 	move(path, filepath.Join(new, after))
// }

func compressWithOpts(gpkg string, name string, table string, optss []CompressOpts) {
	dir, _ := filepath.Split(gpkg)

	fmt.Printf("compress %s\n", name)
	var err error
	anyErrors := false
	optch := make(chan CompressOpts, 1)
	wg := &sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for opts := range optch {
				tgpkg := dir + name + "_" + opts.Name + ".gpkg"
				rtgpkg := dir + name + "_roundtrip_" + opts.Name + ".gpkg"

				err = ifNotExists(tgpkg, func() error {
					return compressGeopackage(gpkg, tgpkg, table, opts)
				})
				if err != nil {
					fmt.Printf("compress %s failed: %s\n", opts.Name, err.Error())
					anyErrors = true
				}

				err = ifNotExists(rtgpkg, func() error {
					return decompressGeopackage(opts.Name, tgpkg, rtgpkg, table)
				})
				if err != nil {
					fmt.Printf("decompress %s failed: %s\n", opts.Name, err.Error())
					anyErrors = true
				}
			}
		}()
	}

	for _, opts := range optss {
		optch <- opts
	}
	close(optch)
	wg.Wait()

	if anyErrors {
		panic(fmt.Errorf("compress %s failed", name))
	}
	fmt.Printf("compress %s done\n", name)
}

// type point struct {
// 	x float64
// 	y float64
// }

type Place struct {
	Name   string
	zoom   float64
	latlng s2.LatLng
	// point
}

type renderJob struct {
	gpkg   string
	table  string
	png    string
	w      int
	h      int
	latlng s2.LatLng
	zoom   float64
}

func renderJobs(name string, table string, ch <-chan renderJob) {
	fmt.Printf("render %s\n", name)

	wg := &sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range ch {
				err := ifNotExists(job.png, func() error {
					return render(job.gpkg, job.table, job.png, job.w, job.h, job.latlng, job.zoom)
				})
				if err != nil {
					panic(err)
				}
			}
		}()
	}
	wg.Wait()
	fmt.Printf("render %s done\n", name)
}

type RenderOpts struct {
	w      int
	h      int
	Places []Place
}

func renderRoundtrip(name string, table string, optss []CompressOpts, ropts RenderOpts) {
	jobs := make(chan renderJob)

	// zooms := []int{0, 1, 2}

	go func() {
		for _, opts := range optss {
			for _, zp := range ropts.Places {
				gpkg := DATA_DIR + name + "_roundtrip_" + opts.Name + ".gpkg"
				png := DATA_DIR + name + "_roundtrip_" + opts.Name + "_" + zp.Name + ".png"
				jobs <- renderJob{
					gpkg:   gpkg,
					table:  table,
					png:    png,
					w:      ropts.w,
					h:      ropts.h,
					latlng: zp.latlng,
					zoom:   float64(zp.zoom),
				}
			}
		}
		close(jobs)
	}()
	renderJobs(name, table, jobs)
}

// func generate(name string, optss []compressOpts) {
// 	fmt.Printf("generate %s\n", name)

// 	geojson := DATA_DIR + name + ".geojson"
// 	gpkg := DATA_DIR + name + ".gpkg"
// 	var err error
// 	err = ifNotExists(geojson, func() error {
// 		return download(geojsonUrl(name), geojson)
// 	})
// 	if err != nil {
// 		panic(err)
// 	}

// 	err = ifNotExists(gpkg, func() error {
// 		return convertToGeopackage(geojson, gpkg)
// 	})
// 	if err != nil {
// 		panic(err)
// 	}

// 	compressWithOpts(gpkg, name, name, optss)

// 	fmt.Printf("generate %s done\n", name)
// }

func optrange(fromPrec int, toPrec int, fromSimp int, toSimp int) []CompressOpts {
	optss := []CompressOpts{}
	// fromPrec, toPrec := 3, 3
	// fromSimp, toSimp := 0, 5
	// fromSimp, toSimp := 1, 4
	for minPrec := fromPrec; minPrec <= toPrec; minPrec++ {
		for i := fromSimp; i <= toSimp; i++ {
			simplify := []float64{}
			jrange := toSimp - fromSimp
			if jrange < 3 {
				jrange = 3
			}
			for j := i; j <= fromSimp+jrange; j++ {
				// simplify = append(simplify, math.Pow(10, float64(-1-j)))
				simplify = append(simplify, math.Pow(10, float64(3-j)))
			}
			optss = append(optss, CompressOpts{
				Name:              fmt.Sprintf("twkb_p%d_s%d", minPrec, i),
				MinPrecXY:         minPrec,
				Simplify:          simplify,
				SimplifyMinPoints: 20,
			})
		}
	}
	return optss
}

func writeMarkdown(w io.Writer, name string, optss []CompressOpts, ropts RenderOpts) {
	fmt.Fprintf(w, `# %s`, name)
	fmt.Fprintln(w)
	fmt.Fprintf(w, `| variant | file size | `)
	for _, place := range ropts.Places {
		fmt.Fprintf(w, `%s | `, place.Name)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, `| --- | --- | `)
	for range ropts.Places {
		fmt.Fprintf(w, `--- | `)
	}
	fmt.Fprintln(w)

	for _, opts := range optss {
		size := "❓"
		gpkg := name + "_" + opts.Name + ".gpkg"
		stat, err := os.Stat(DATA_DIR + gpkg)
		if err == nil {
			size = fmt.Sprintf("%d KB", stat.Size()/1000)
		}
		fmt.Fprintf(
			w,
			"| [%s](%s) | %s |",
			opts.Name,
			"data/"+gpkg,
			size,
		)
		for _, place := range ropts.Places {
			fmt.Fprintf(w,
				`<img src="data/%s_roundtrip_%s_%s.png" alt="%s"> | `,
				name, opts.Name, place.Name, opts.Name+" "+place.Name,
			)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)

	// | --- | --- | --- | --- | --- | --- | --- | --- |
	// | ne_110m_admin_0_countries | twkb_p3_s0 | <img src="data/ne_110m_admin_0_countries_roundtrip_twkb_p3_s2_p0.png" alt="ne_110m_admin_0_countries" width="50%"> |  <img src="data/ne_110m_admin_0_countries_roundtrip_twkb_p3_s2_p1.png" alt="ne_110m_admin_0_countries" width="100%"> |  <img src="data/ne_110m_admin_0_countries_roundtrip_twkb_p3_s2_p2.png" alt="ne_110m_admin_0_countries" width="100%"> |  <img src="data/ne_110m_admin_0_countries_roundtrip_twkb_p3_s2_p3.png" alt="ne_110m_admin_0_countries" width="100%"> |  <img src="data/ne_110m_admin_0_countries_roundtrip_twkb_p3_s2_p4.png" alt="ne_110m_admin_0_countries" width="100%"> |

	// 	// Markdown table header
	// 	fmt.Fprintf(w, "| Compression Type | Image |\n")
	// 	fmt.Fprintf(w, "| --- | --- |\n")

}

type Readme struct {
	Parameters []CompressOpts
	Datasets   []Dataset
}

type Dataset struct {
	Name     string
	Variants []CompressOpts
	Render   RenderOpts
	// Variants []Variant
}

type Variant struct {
	Name     string
	Path     string
	Size     string
	Previews []Preview
}

type Preview struct {
	Name string
	Path string
}

func datasets(name string, optss []CompressOpts, ropts RenderOpts) {
	// fmt.Fprintf(w, `# %s`, name)
	// fmt.Fprintln(w)
	// fmt.Fprintf(w, `| variant | file size | `)
	// for _, place := range ropts.places {
	// 	fmt.Fprintf(w, `%s | `, place.name)
	// }
	// fmt.Fprintln(w)

	// fmt.Fprintf(w, `| --- | --- | `)
	// for range ropts.places {
	// 	fmt.Fprintf(w, `--- | `)
	// }
	// fmt.Fprintln(w)

	// for _, opts := range optss {
	// 	size := "❓"
	// 	gpkg := name + "_" + opts.name + ".gpkg"
	// 	stat, err := os.Stat(DATA_DIR + gpkg)
	// 	if err == nil {
	// 		size = fmt.Sprintf("%d KB", stat.Size()/1000)
	// 	}
	// 	fmt.Fprintf(
	// 		w,
	// 		"| [%s](%s) | %s |",
	// 		opts.name,
	// 		"data/"+gpkg,
	// 		size,
	// 	)
	// 	for _, place := range ropts.places {
	// 		fmt.Fprintf(w,
	// 			`<img src="data/%s_roundtrip_%s_%s.png" alt="%s"> | `,
	// 			name, opts.name, place.name, opts.name+" "+place.name,
	// 		)
	// 	}
	// 	fmt.Fprintln(w)
	// }
	// fmt.Fprintln(w)

	// | --- | --- | --- | --- | --- | --- | --- | --- |
	// | ne_110m_admin_0_countries | twkb_p3_s0 | <img src="data/ne_110m_admin_0_countries_roundtrip_twkb_p3_s2_p0.png" alt="ne_110m_admin_0_countries" width="50%"> |  <img src="data/ne_110m_admin_0_countries_roundtrip_twkb_p3_s2_p1.png" alt="ne_110m_admin_0_countries" width="100%"> |  <img src="data/ne_110m_admin_0_countries_roundtrip_twkb_p3_s2_p2.png" alt="ne_110m_admin_0_countries" width="100%"> |  <img src="data/ne_110m_admin_0_countries_roundtrip_twkb_p3_s2_p3.png" alt="ne_110m_admin_0_countries" width="100%"> |  <img src="data/ne_110m_admin_0_countries_roundtrip_twkb_p3_s2_p4.png" alt="ne_110m_admin_0_countries" width="100%"> |

	// 	// Markdown table header
	// 	fmt.Fprintf(w, "| Compression Type | Image |\n")
	// 	fmt.Fprintf(w, "| --- | --- |\n")

}

func main() {

	opts := optrange(3, 3, 3, 8)
	// opts := optrange(3, 3, 3, 8)
	ropts := RenderOpts{
		w: 512, h: 512,
		Places: []Place{
			{"world", 0, s2.LatLngFromDegrees(0, 0)},
			{"europe", 3, s2.LatLngFromDegrees(49, 9.6)},
			{"africa", 2, s2.LatLngFromDegrees(6, 19)},
			{"usa", 3, s2.LatLngFromDegrees(40, -101)},
			{"japan", 5, s2.LatLngFromDegrees(35, 130)},
		},
	}

	cityropts := RenderOpts{
		w: 512, h: 512,
		Places: []Place{
			{"world", 0, s2.LatLngFromDegrees(0, 0)},
			{"berlin", 9, s2.LatLngFromDegrees(52.44504, 13.40973)},
			{"nyc", 9, s2.LatLngFromDegrees(40.76828, -73.88639)},
			{"tokyo", 8, s2.LatLngFromDegrees(35.7295, 139.70422)},
			{"ljubljana", 10, s2.LatLngFromDegrees(46.09049, 14.54004)},
			// {"atacama", 10, s2.LatLngFromDegrees(-24.8088, -65.4237)},
		},
	}

	t, err := template.
		New("readme").
		Funcs(template.FuncMap{
			"filesize": func(path string) string {
				stat, err := os.Stat(OUTPUT_DIR + path)
				if err != nil {
					return "❓"
				}
				return fmt.Sprintf("%d KB", stat.Size()/1000)
			},
		}).
		Parse(readmeTemplate)

	if err != nil {
		panic(err)
	}

	f, err := os.Create(OUTPUT_DIR + "README.md")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	f.WriteString("<!-- Generated from README.tmpl.md DO NOT EDIT -->\n\n")

	readme := Readme{
		Parameters: opts,
	}

	download(NATURAL_EARTH_URL+"ne_110m_admin_0_countries.geojson", DATA_DIR+"ne_110m_admin_0_countries.geojson")
	convertToGeopackage(DATA_DIR+"ne_110m_admin_0_countries.geojson", DATA_DIR+"ne_110m_admin_0_countries_valid.gpkg")
	compressWithOpts(DATA_DIR+"ne_110m_admin_0_countries_valid.gpkg", "ne_110m_admin_0_countries", "ne_110m_admin_0_countries", opts)
	renderRoundtrip("ne_110m_admin_0_countries", "ne_110m_admin_0_countries", opts, ropts)
	readme.Datasets = append(readme.Datasets, Dataset{
		Name:     "ne_110m_admin_0_countries",
		Variants: opts,
		Render:   ropts,
	})

	download(NATURAL_EARTH_URL+"ne_10m_admin_0_countries.geojson", DATA_DIR+"ne_10m_admin_0_countries.geojson")
	convertToGeopackage(DATA_DIR+"ne_10m_admin_0_countries.geojson", DATA_DIR+"ne_10m_admin_0_countries_valid.gpkg")
	compressWithOpts(DATA_DIR+"ne_10m_admin_0_countries_valid.gpkg", "ne_10m_admin_0_countries", "ne_10m_admin_0_countries", opts)
	renderRoundtrip("ne_10m_admin_0_countries", "ne_10m_admin_0_countries", opts, ropts)
	readme.Datasets = append(readme.Datasets, Dataset{
		Name:     "ne_10m_admin_0_countries",
		Variants: opts,
		Render:   ropts,
	})

	download(NATURAL_EARTH_URL+"ne_10m_urban_areas_landscan.geojson", DATA_DIR+"ne_10m_urban_areas_landscan.geojson")
	convertToGeopackage(DATA_DIR+"ne_10m_urban_areas_landscan.geojson", DATA_DIR+"ne_10m_urban_areas_landscan_valid.gpkg")
	compressWithOpts(DATA_DIR+"ne_10m_urban_areas_landscan_valid.gpkg", "ne_10m_urban_areas_landscan", "ne_10m_urban_areas_landscan", opts)
	renderRoundtrip("ne_10m_urban_areas_landscan", "ne_10m_urban_areas_landscan", opts, cityropts)
	readme.Datasets = append(readme.Datasets, Dataset{
		Name:     "ne_10m_urban_areas_landscan",
		Variants: opts,
		Render:   cityropts,
	})

	download(GEOBOUNDARIES_URL+"CGAZ/geoBoundariesCGAZ_ADM2.gpkg", DATA_DIR+"geoBoundariesCGAZ_ADM2.gpkg")
	convertToGeopackage(DATA_DIR+"geoBoundariesCGAZ_ADM2.gpkg", DATA_DIR+"geoBoundariesCGAZ_ADM2_valid.gpkg")
	compressWithOpts(DATA_DIR+"geoBoundariesCGAZ_ADM2_valid.gpkg", "geoBoundariesCGAZ_ADM2", "globalADM2", opts)
	renderRoundtrip("geoBoundariesCGAZ_ADM2", "globalADM2", opts, ropts)
	readme.Datasets = append(readme.Datasets, Dataset{
		Name:     "geoBoundariesCGAZ_ADM2",
		Variants: opts,
		Render:   ropts,
	})

	// Invalid geometry breaks some of the processes currently
	// download(GEOBOUNDARIES_URL+"CGAZ/geoBoundariesCGAZ_ADM1.gpkg", DATA_DIR+"geoBoundariesCGAZ_ADM1.gpkg")
	// convertToGeopackage(DATA_DIR+"geoBoundariesCGAZ_ADM1.gpkg", DATA_DIR+"geoBoundariesCGAZ_ADM1_valid.gpkg")
	// compressWithOpts(DATA_DIR+"geoBoundariesCGAZ_ADM1_valid.gpkg", "geoBoundariesCGAZ_ADM1", "globalADM1", opts)
	// renderRoundtrip("geoBoundariesCGAZ_ADM1", "globalADM1", opts, ropts)
	// readme.Datasets = append(readme.Datasets, Dataset{
	// 	Name:     "geoBoundariesCGAZ_ADM1",
	// 	Variants: opts,
	// 	Render:   ropts,
	// })

	download(GEOBOUNDARIES_URL+"CGAZ/geoBoundariesCGAZ_ADM0.gpkg", DATA_DIR+"geoBoundariesCGAZ_ADM0.gpkg")
	convertToGeopackage(DATA_DIR+"geoBoundariesCGAZ_ADM0.gpkg", DATA_DIR+"geoBoundariesCGAZ_ADM0_valid.gpkg")
	compressWithOpts(DATA_DIR+"geoBoundariesCGAZ_ADM0_valid.gpkg", "geoBoundariesCGAZ_ADM0", "globalADM0", opts)
	renderRoundtrip("geoBoundariesCGAZ_ADM0", "globalADM0", opts, ropts)
	readme.Datasets = append(readme.Datasets, Dataset{
		Name:     "geoBoundariesCGAZ_ADM0",
		Variants: opts,
		Render:   ropts,
	})

	err = t.Execute(f, readme)
	if err != nil {
		panic(err)
	}

}
