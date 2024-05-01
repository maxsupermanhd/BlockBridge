package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

func SetupDatabase() *pgxpool.Pool {
	db := noerr(pgxpool.Connect(context.Background(), cfg.GetDString("", "DatabaseConnection")))
	noerr(db.Exec(context.Background(), `create table if not exists tps (whenlogged timestamp, tpsvalue real, playercount bigint, worldage bigint);`))
	noerr(db.Exec(context.Background(), `create index if not exists tps_index on tps (whenlogged);`))
	noerr(db.Exec(context.Background(), `create table if not exists backonlinesubs(discorduserid text not null primary key, subtime int not null, dmchanid text not null);`))
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

type pingbackonline struct {
	discorduserid string
	dmchannelid   string
	subtime       int
}

func removePingbackonlineSub(db *pgxpool.Pool, userid string) (string, error) {
	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second*3)
	defer ctxCancel()
	result, err := db.Exec(ctx, `delete from backonlinesubs where discorduserid = $1;`, userid)
	return result.String(), err
}

func recordPingbackonlineSub(db *pgxpool.Pool, p pingbackonline) (string, error) {
	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second*3)
	defer ctxCancel()
	result, err := db.Exec(ctx, `insert into backonlinesubs (discorduserid, subtime, dmchanid) values($1, $2, $3) on conflict (discorduserid) do update set subtime = excluded.subtime;`,
		p.discorduserid, p.subtime, p.dmchannelid)
	return result.String(), err
}

func getNextPingbackonlineSub(db *pgxpool.Pool) (*pingbackonline, error) {
	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second*3)
	defer ctxCancel()
	var discorduserid, dmchanid string
	var subtime int
	err := db.QueryRow(ctx, `select discorduserid, subtime, dmchanid from backonlinesubs order by subtime asc limit 1;`).Scan(&discorduserid, &subtime, &dmchanid)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &pingbackonline{
		discorduserid: discorduserid,
		dmchannelid:   dmchanid,
		subtime:       subtime,
	}, err
}
