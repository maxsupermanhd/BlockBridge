package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"
)

func SetupDatabase() *sql.DB {
	db := noerr(sql.Open("sqlite3", loadedConfig.DatabaseFile))
	noerr(db.Exec(`create table if not exists tps (whenlogged timestamp, tpsvalue float);`))
	noerr(db.Exec(`create index if not exists tps_index on tps (whenlogged);`))
	noerr(db.Exec(`create table if not exists lagspikes (whenlogged timestamp, tpsprev float, tpscurrent float, players text);`))
	noerr(db.Exec(`create index if not exists lagspikes_index on lagspikes (whenlogged);`))
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
	rows := noerr(db.Query(`select strftime('%Y-%m-%d %H:%M:%S', datetime(whenlogged, 'unixepoch')) , tpsvalue from tps order by whenlogged desc limit 500;`))
	fmt.Fprint(buff, "Last 500 TPS samples:\n")
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

type lagspike struct {
	timestamp   string
	playersJSON string
	tpsval0     float64
	tpsval1     float64
}

func FormatLagspikes(s []lagspike) *bytes.Buffer {
	buff := bytes.NewBufferString(fmt.Sprintf("Lagspikes (%d):\n", len(s)))
	fmt.Fprint(buff, "Timestamp (UTC), TPS, players\n")
	for _, v := range s {
		fmt.Fprintf(buff, "%s, %.2f -> %.2f, %s\n", v.timestamp, v.tpsval0, v.tpsval1, v.playersJSON)
	}
	return buff
}

func GetLastLagspikes(db *sql.DB, dur *time.Duration) ([]lagspike, error) {
	var rows *sql.Rows
	var err error
	ret := []lagspike{}
	if dur != nil {
		time.Now().Add(-dur.Abs()).Unix()
		rows, err = db.Query(`select strftime('%Y-%m-%d %H:%M:%S', datetime(whenlogged, 'unixepoch')), tpsprev, tpscurrent, players from lagspikes where whenlogged > $1 order by whenlogged desc;`, time.Now().Add(-dur.Abs()).Unix())
	} else {
		rows, err = db.Query(`select strftime('%Y-%m-%d %H:%M:%S', datetime(whenlogged, 'unixepoch')), tpsprev, tpscurrent, players from lagspikes order by whenlogged desc limit 150;`)
	}
	defer rows.Close()
	if err != nil {
		return ret, err
	}
	for rows.Next() {
		var tmstp, pl string
		var tpsval0, tpsval1 float64
		err := rows.Scan(&tmstp, &tpsval0, &tpsval1, &pl)
		if err != nil {
			return ret, err
		}
		ret = append(ret, lagspike{
			timestamp:   tmstp,
			playersJSON: pl,
			tpsval0:     tpsval0,
			tpsval1:     tpsval1,
		})
	}
	return ret, err
}

func GetRankedLagspikes(spikes []lagspike) (*bytes.Buffer, error) {
	buff := bytes.NewBufferString("")
	l := map[string]int{}
	c := 0
	for _, s := range spikes {
		var p []string
		err := json.Unmarshal([]byte(s.playersJSON), &p)
		if err != nil {
			return buff, err
		}
		for _, v := range p {
			c, ok := l[v]
			if ok {
				l[v] = c + 1
			} else {
				l[v] = 1
			}
		}
		c++
	}
	fmt.Fprintf(buff, "Rank of uuids that were online while server lagspiked: (%d lagspikes)\n", c)
	fmt.Fprint(buff, "uuid, count of lagspikes\n")
	ll := []struct {
		u string
		c int
	}{}
	for k, v := range l {
		ll = append(ll, struct {
			u string
			c int
		}{
			u: k,
			c: v,
		})
	}
	sort.Slice(ll, func(i, j int) bool {
		return ll[i].c > ll[j].c
	})
	num := 0
	for _, v := range ll {
		if num == 150 {
			break
		}
		fmt.Fprintf(buff, "%s %d\n", v.u, v.c)
		num++
	}
	return buff, nil
}
