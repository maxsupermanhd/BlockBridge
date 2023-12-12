package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)

func SetupDatabase() *pgxpool.Pool {
	db := noerr(pgxpool.Connect(context.Background(), cfg.GetDString("", "DatabaseConnection")))
	noerr(db.Exec(context.Background(), `create table if not exists tps (whenlogged timestamp, tpsvalue float, playercount integer);`))
	noerr(db.Exec(context.Background(), `create index if not exists tps_index on tps (whenlogged);`))
	// noerr(db.Exec(context.Background(), `create table if not exists lagspikes (whenlogged timestamp, tpsprev float, tpscurrent float, players text);`))
	// noerr(db.Exec(context.Background(), `create index if not exists lagspikes_index on lagspikes (whenlogged);`))
	return db
}

func GetTPSValues(db *pgxpool.Pool, t *time.Duration) ([]time.Time, []float64, error) {
	tv := time.Duration(24 * time.Hour)
	if t != nil {
		tv = *t
	}
	rows, err := db.Query(context.Background(), `select whenlogged, tpsvalue from tps where whenlogged + $1::interval > now() order by whenlogged asc;`, fmt.Sprintf("%d seconds", int(math.Round(tv.Seconds()))))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	tpsval := []time.Time{}
	tpsn := []float64{}
	for rows.Next() {
		var (
			tpsunix time.Time
			tps     float64
		)
		err = rows.Scan(&tpsunix, &tps)
		if err != nil {
			return nil, nil, err
		}
		tpsavgs := float64(0)
		tpsavgc := float64(0)
		timeavg := 20
		ticksavg := timeavg * 20
		for i := len(tpsn); i > 0 && i+ticksavg < len(tpsn); i++ {
			if tpsunix.Sub(tpsval[i]) > time.Duration(timeavg)*time.Second {
				break
			}
			tpsavgc++
			tpsavgs += tpsn[i]
		}
		tpsval = append(tpsval, tpsunix)
		if tpsavgc > 0 {
			tpsn = append(tpsn, tpsavgs/tpsavgc)
		} else {
			tpsn = append(tpsn, tps)
		}
	}
	return tpsval, tpsn, nil
}

func GetTPSPlayercountValues(db *pgxpool.Pool, t *time.Duration) ([]time.Time, []float64, []float64, error) {
	tv := time.Duration(2 * 24 * time.Hour)
	if t != nil {
		tv = *t
	}
	rows, err := db.Query(context.Background(), `select whenlogged, tpsvalue, playercount from tps where whenlogged + $1::interval > now() order by whenlogged asc;`, fmt.Sprintf("%d seconds", int(math.Round(tv.Seconds()))))
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	tpsval := []time.Time{}
	tpsn := []float64{}
	playercountn := []float64{}
	for rows.Next() {
		var (
			tpsunix     time.Time
			tps         float64
			playercount int
		)
		err = rows.Scan(&tpsunix, &tps, &playercount)
		if err != nil {
			return nil, nil, nil, err
		}
		tpsavgs := float64(0)
		tpsavgc := float64(0)
		timeavg := 20
		ticksavg := timeavg * 20
		for i := len(tpsn); i > 0 && i+ticksavg < len(tpsn); i++ {
			if tpsunix.Sub(tpsval[i]) > time.Duration(timeavg)*time.Second {
				break
			}
			tpsavgc++
			tpsavgs += tpsn[i]
		}
		tpsval = append(tpsval, tpsunix)
		if tpsavgc > 0 {
			tpsn = append(tpsn, tpsavgs/tpsavgc)
		} else {
			tpsn = append(tpsn, tps)
		}
		playercountn = append(playercountn, float64(playercount))
	}
	return tpsval, tpsn, playercountn, nil
}

func GetLastTPSValues(db *pgxpool.Pool) *bytes.Buffer {
	buff := bytes.NewBufferString("")
	rows := noerr(db.Query(context.Background(), `select whenlogged, tpsvalue from tps order by whenlogged desc limit 500;`))
	fmt.Fprint(buff, "Last 500 TPS samples:\n")
	fmt.Fprint(buff, "Timestamp (UTC), TPS\n")
	for rows.Next() {
		var tmstp time.Time
		var tpsval float64
		err := rows.Scan(&tmstp, &tpsval)
		if err != nil {
			log.Println(err)
			break
		}
		fmt.Fprintf(buff, "%s, %.2f\n", tmstp.Format(time.DateTime), tpsval)
	}
	rows.Close()
	return buff
}

func getAvgPlayercountLong(db *pgxpool.Pool) ([]time.Time, []float64, error) {
	rows, err := db.Query(context.Background(), `select s+'5 minutes'::interval, avg(playercount) from generate_series(now() - '14 days'::interval, now(), '10 minutes') as s, tps where whenlogged >= s and whenlogged <= s+'10 minutes'::interval group by s+'5 minutes'::interval order by s+'5 minutes'::interval;`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	rett := []time.Time{}
	retv := []float64{}
	for rows.Next() {
		var t time.Time
		var v float64
		err = rows.Scan(&t, &v)
		if err != nil {
			return nil, nil, err
		}
		rett = append(rett, t)
		retv = append(retv, v)
	}
	return rett, retv, nil
}
