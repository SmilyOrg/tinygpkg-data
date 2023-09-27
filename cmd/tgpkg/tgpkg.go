package main

import (
	"bytes"
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/peterstace/simplefeatures/geom"
	"github.com/smilyorg/tinygpkg/binary"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func Build() error {
	println("hello world")
	return nil
}

const OUTPUT_DIR = "./"
const DATA_DIR = OUTPUT_DIR + "data/"
const NATURAL_EARTH_URL = "https://raw.githubusercontent.com/nvkelso/natural-earth-vector/117488dc884bad03366ff727eca013e434615127/geojson/"
const GEOBOUNDARIES_URL = "https://github.com/wmgeolab/geoBoundaries/raw/742b9ae7d6a57e57fe6d47f29343bc1081a8f09d/releaseData/"
const GDAL_DOCKER_IMAGE = "ghcr.io/osgeo/gdal:alpine-normal-3.7.1"
const MIN_PRECXY = -8
const MAX_PRECXY = +7

type Opts struct {
	MinPrecXY int
	MaxPrecXY int
}

type compressJob struct {
	fid int64
	wkb []byte
	h   binary.Header
}

type compressInfo struct {
	debug        string
	usedPrecXY   int
	originalSize int
}

type writeJob struct {
	name string
	fid  int64
	twkb []byte
	h    binary.Header
	info compressInfo
}

func compressor(name string, wg *sync.WaitGroup, in <-chan compressJob, out chan<- writeJob, opts Opts) {
	for job := range in {
		twkb, info, err := compressGeometry(job.wkb, opts)
		if err != nil {
			panic(err)
		}
		if twkb == nil {
			continue
		}
		out <- writeJob{name, job.fid, twkb, job.h, info}
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

		g.SetType(binary.ExtendedType)
		g.ExtensionCode = binary.ExtensionTWKB
		g.SetEnvelopeContentsIndicatorCode(binary.NoEnvelope)

		// Write TWKB
		w := new(bytes.Buffer)
		g.Write(w)
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
			"compress %10s fid %6d %7d wkb bytes %7d twkb bytes %4.0f%% at %d precXY %6d bytes written %s\n",
			job.name, fid, info.originalSize, len(twkb), 100.*float32(len(twkb))/float32(info.originalSize), info.usedPrecXY, len(buf), info.debug,
		)
	}
	wg.Done()
}

func compressGeopackage(name, gpkgPath, tgpkgPath, table string, opts Opts) error {
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
		go compressor(name, cwg, compressChan, writeChan, opts)
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
		g, err := binary.Read(r)
		if err != nil {
			return fmt.Errorf("error reading gpkg: %s", err.Error())
		}

		if g.Empty() {
			fmt.Printf("compress fid %6d empty, skipping\n", fid)
			continue
		}

		if g.Type() != binary.StandardType {
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

func compressGeometry(wkb []byte, opts Opts) (twkb []byte, info compressInfo, err error) {
	info.originalSize = len(wkb)

	gm, err := geom.UnmarshalWKB(wkb)
	if err != nil {
		return nil, info, fmt.Errorf("error unmarshalling wkb: %s", err.Error())
	}

	var extra []string

	info.usedPrecXY = opts.MinPrecXY
	for ; info.usedPrecXY <= opts.MaxPrecXY; info.usedPrecXY++ {
		twkb, err = geom.MarshalTWKB(gm, info.usedPrecXY)
		if err != nil {
			return nil, info, fmt.Errorf("error marshalling twkb: %s", err.Error())
		}

		_, err = geom.UnmarshalTWKB(twkb)
		if err == nil {
			break
		}
	}

	if info.usedPrecXY >= opts.MaxPrecXY {
		extra = append(extra, "unable to compress, skipping")
		twkb = nil
	} else if info.usedPrecXY != opts.MinPrecXY {
		extra = append(extra, "roundtrip fallback")
	}

	info.debug = strings.Join(extra, " ")
	return twkb, info, nil
}

func decompressGeopackage(name, tgpkgPath, gpkgPath, table string) error {
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
		g, err := binary.Read(r)
		if err != nil {
			return fmt.Errorf("unable to read gpkg: %w", err)
		}

		if g.Empty() {
			fmt.Printf("decompress fid %6d empty, skipping\n", fid)
			continue
		}

		if g.Type() == binary.StandardType {
			fmt.Printf("decompress fid %6d standard geometry, skipping\n", fid)
			continue
		}

		if !bytes.Equal(g.ExtensionCode, binary.ExtensionTWKB) {
			return fmt.Errorf("invalid extension code: %s", string(g.ExtensionCode))
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

		g.SetType(binary.StandardType)
		g.SetEnvelopeContentsIndicatorCode(binary.NoEnvelope)

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

type Place struct {
	Name string
}

func tempCopy(src string, dst string) (string, func(), error) {
	tmp := dst + ".tmp"
	err := exec.Command("cp", src, tmp).Run()
	if err != nil {
		return tmp, nil, err
	}

	return tmp, func() {
		exec.Command("mv", tmp, dst).Run()
	}, nil
}

func main() {
	table := flag.String("table", "ne_110m_admin_0_countries", "table to compress")
	minprecxy := flag.Int("minprecxy", 3, "minimum X and Y coordinate TWKB precision")
	maxprecxy := flag.Int("maxprecxy", MAX_PRECXY, "maximum X and Y coordinate TWKB precision")
	output := flag.String("o", "", "output file name")
	flag.Parse()

	if len(flag.Args()) < 2 {
		fmt.Println("Error: command and input file name are required")
		return
	}

	cmd := flag.Arg(0)
	path := flag.Arg(1)

	if *output == "" {
		fmt.Println("Error: output file name is required")
		return
	}

	basename := filepath.Base(*output)

	tmp, move, err := tempCopy(path, *output)
	if err != nil {
		fmt.Println("Error: unable to copy file")
		return
	}
	defer move()

	opts := Opts{
		MinPrecXY: *minprecxy,
		MaxPrecXY: *maxprecxy,
	}

	switch cmd {
	case "compress":
		compressGeopackage(basename, path, tmp, *table, opts)
	case "decompress":
		decompressGeopackage(basename, path, tmp, *table)
	default:
		fmt.Println("Error: unknown command: " + cmd)
	}
}
