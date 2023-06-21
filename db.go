package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"time"
)

func SetupDatabase() *sql.DB {
	db := noerr(sql.Open("sqlite3", loadedConfig.DatabaseFile))
	noerr(db.Exec(`create table if not exists tps (whenlogged timestamp, tpsvalue float);`))
	noerr(db.Exec(`create index if not exists tps_index on tps (whenlogged);`))
	return db
}

func GetTPSValues(db *sql.DB) ([]time.Time, []float64, error) {
	rows, err := db.Query(`select cast(whenlogged as int), tpsvalue from tps where whenlogged + 24*60*60 > unixepoch() order by whenlogged asc;`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	tpsval := []time.Time{}
	tpsn := []float64{}
	for rows.Next() {
		var (
			when int64
			tps  float64
		)
		err = rows.Scan(&when, &tps)
		if err != nil {
			return nil, nil, err
		}
		tpsunix := time.Unix(when, 0)
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

func GetLastTPSValues(db *sql.DB) *bytes.Buffer {
	buff := bytes.NewBufferString("")
	rows := noerr(db.Query(`select strftime('%Y-%m-%d %H:%M:%S', datetime(whenlogged, 'unixepoch')) , tpsvalue from tps order by whenlogged desc limit 50;`))
	fmt.Fprint(buff, "Last 50 TPS samples:\n")
	fmt.Fprint(buff, "Timestamp (UTC), TPS\n")
	for rows.Next() {
		var tmstp string
		var tpsval float64
		err := rows.Scan(&tmstp, &tpsval)
		if err != nil {
			log.Println(err)
			break
		}
		fmt.Fprintf(buff, "%s, %.2f\n", tmstp, tpsval)
	}
	rows.Close()
	return buff
}
