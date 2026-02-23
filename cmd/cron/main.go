package main

import (
	"liquid8/wms/cmd/jobs"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"time"

	// "time"

	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
)

func main() {
	// logger umum
	log := helpers.NewLogger("./logs/cronjob.log")
	config.InitDB()

	loc, _ := time.LoadLocation("Asia/Jakarta")
	c := cron.New(cron.WithLocation(loc), cron.WithSeconds())

	/*
		┌──────── second (0 - 59)
		│ ┌────── minute
		│ │ ┌──── hour
		│ │ │ ┌── day of month
		│ │ │ │ ┌ month
		│ │ │ │ │ ┌ day of week
		│ │ │ │ │ │
		* * * * * *
	*/

	//setiap hari jam 9 malam -> jalankan daily snapshoot
	c.AddFunc("0 0 21 * * *", safeJob(jobs.RunSummaryDaily, log))
	c.AddFunc("0 0 21 * * *", safeJob(jobs.RunDailySnapshot, log))
	//setiap hari
	c.AddFunc("0 0 0 * * *", safeJob(jobs.RunExpirdBuyerLoyalty, log))
	c.AddFunc("0 0 0 * * *", safeJob(jobs.RunExpireProducts, log))
	c.AddFunc("0 0 0 * * *", safeJob(jobs.RunSlowMovingProduct, log))
	//Jadwalkan command untuk dijalankan pada pukul 23.50 pada hari terakhir bulan
	c.AddFunc("0 50 23 * * *", safeJob(jobs.RunEndOfMonthTask, log))


	log.Info("Cron Worker started")
	c.Start()
	select {}
}

func safeJob(job func(*logrus.Logger), log *logrus.Logger) func() {
	return func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("CRON PANIC RECOVERED:", r)
			}
		}()
		job(log)
	}
}