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
	"strconv"
	"strings"
	"sync"

	"github.com/peterstace/simplefeatures/geom"
	"github.com/smilyorg/tinygpkg/binary"
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

type Opts struct {
	Levels    []float64
	MinPoints int
}

type simplifyJob struct {
	name string
	fid  int64
	wkb  []byte
	h    binary.Header
}

type Result struct {
	debug            string
	usedSimplify     float64
	originalSize     int
	originalPoints   int
	simplifiedPoints int
}

type writeJob struct {
	name string
	fid  int64
	wkb  []byte
	h    binary.Header
	info Result
}

func simplifier(wg *sync.WaitGroup, in <-chan simplifyJob, out chan<- writeJob, opts Opts) {
	for job := range in {
		simplified, info, err := simplifyGeometry(job.wkb, opts)
		if err != nil {
			panic(err)
		}
		if simplified == nil {
			continue
		}
		out <- writeJob{job.name, job.fid, simplified, job.h, info}
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
		wkb := job.wkb
		info := job.info

		g.SetEnvelopeContentsIndicatorCode(binary.NoEnvelope)

		// Write simplified
		w := new(bytes.Buffer)
		g.Write(w)
		io.Copy(w, bytes.NewReader(wkb))

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
			"simplify %10s fid %6d %.0e simplify %6d to %6d points %7d to %7d bytes %4.0f%% at %6d bytes written %s\n",
			job.name, fid, info.usedSimplify, info.originalPoints, info.simplifiedPoints, info.originalSize, len(wkb), 100.*float32(len(wkb))/float32(info.originalSize), len(buf), info.debug,
		)
	}
	wg.Done()
}

func simplify(name string, path string, table string, opts Opts) error {
	pool, err := sqlitex.Open(path, 0, 2)
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
		DROP TRIGGER IF EXISTS rtree_%[1]s_geom_update1;
		DROP TRIGGER IF EXISTS rtree_%[1]s_geom_update2;
		DROP TRIGGER IF EXISTS rtree_%[1]s_geom_update3;
		DROP TRIGGER IF EXISTS rtree_%[1]s_geom_update4;
	`, table), nil)
	if err != nil {
		return err
	}

	sqlitex.Execute(writec, "BEGIN TRANSACTION;", nil)

	simplifyChan := make(chan simplifyJob, 10)
	writeChan := make(chan writeJob, 10)
	cwg := &sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		cwg.Add(1)
		go simplifier(cwg, simplifyChan, writeChan, opts)
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
			fmt.Printf("simplify %s fid %6d empty, skipping\n", table, fid)
			continue
		}

		wkb, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("error reading geometry: %s", err.Error())
		}

		simplifyChan <- simplifyJob{
			name: name,
			fid:  fid,
			wkb:  wkb,
			h:    *g,
		}
	}

	close(simplifyChan)
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

func simplifyGeometry(wkb []byte, opts Opts) (simplified []byte, r Result, err error) {
	r.originalSize = len(wkb)

	gm, err := geom.UnmarshalWKB(wkb)
	if err != nil {
		return nil, r, fmt.Errorf("error unmarshalling wkb: %s", err.Error())
	}

	var extra []string

	r.originalPoints = gm.DumpCoordinates().Length()
	r.simplifiedPoints = r.originalPoints
	r.usedSimplify = 0.0
	if r.originalPoints < opts.MinPoints {
		extra = append(extra, "not simplified min points")
	} else {
		si := 0
		for ; si < len(opts.Levels); si++ {
			r.usedSimplify = opts.Levels[si]
			gms, err := gm.Simplify(r.usedSimplify)
			if err == nil {
				points := gms.DumpCoordinates().Length()
				if points >= 3 {
					gm = gms
					r.simplifiedPoints = points
					break
				}
				extra = append(extra, "not simplified collapse")
				break
			}
		}
		if si == len(opts.Levels) {
			extra = append(extra, "not simplified max level")
		} else if si > 0 {
			extra = append(extra, "simplify fallback")
		}
	}

	simplified = gm.AsBinary()
	r.debug = strings.Join(extra, " ")
	return
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
	table := flag.String("table", "ne_110m_admin_0_countries", "name of the table to simplify")
	levelsString := flag.String("levels", "1,0.1,0.01,0.001", "comma-separated list of simplification levels")
	minPoints := flag.Int("minpoints", 20, "minimum number of points to simplify")
	output := flag.String("o", "", "output file name")
	flag.Parse()

	levels := make([]float64, 0)
	for _, level := range strings.Split(*levelsString, ",") {
		if levelFloat, err := strconv.ParseFloat(level, 64); err == nil {
			levels = append(levels, levelFloat)
		}
	}

	opts := Opts{
		Levels:    levels,
		MinPoints: *minPoints,
	}

	if len(flag.Args()) == 0 {
		fmt.Println("Error: path to the geopackage file is required")
		return
	}

	path := flag.Arg(0)

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
	simplify(basename, tmp, *table, opts)
}
