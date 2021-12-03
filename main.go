package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
	uuid "github.com/satori/go.uuid"
)

const port = 80
const errorPattern = "error: %s"

type application struct {
	router    *mux.Router
	db        *sql.DB
	wait      time.Duration
	templates map[string]*template.Template
}

type homeData struct {
}

type applicationData struct {
}

func initDatabase() *sql.DB {
	scripts := []string{
		`
			PRAGMA foreign_keys = ON;
		`,
		`
			create table if not exists registration (
				id text primary key,
				name string not null,
				phone string not null,
                created_on integer not null
			) without rowid;
		`,
	}

	db, err := sql.Open("sqlite3", "./database.db")
	if err != nil {
		panic(err)
	}

	for _, oneScript := range scripts {
		stmt, err := db.Prepare(oneScript)
		if err != nil {
			panic(err)
		}

		_, err = stmt.Exec()
		if err != nil {
			panic(err)
		}
	}

	return db
}

func main() {
	homeTmpl, err := createTemplate("home", "templates/index.html")
	if err != nil {
		panic(err)
	}

	registrationTmpl, err := createTemplate("registration", "templates/registration.html")
	if err != nil {
		panic(err)
	}

	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish - e.g. 15s or 1m")
	flag.Parse()

	r := mux.NewRouter()
	app := &application{
		router: r,
		db:     initDatabase(),
		wait:   wait,
		templates: map[string]*template.Template{
			"home":         homeTmpl,
			"registration": registrationTmpl,
		},
	}

	r.HandleFunc("/", app.HomeHandler)
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./static/")))

	app.start()
}

func createTemplate(name string, path string) (*template.Template, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return template.New(name).Parse(string(data))
}

// HomeHandler represents the home page
func (app *application) HomeHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		panic(err)
	}

	name := r.FormValue("name")
	phone := r.FormValue("phone")

	if name != "" && phone != "" {
		stmt, err := app.db.Prepare("insert into registration (id, name, phone, created_on) values(?,?,?,?)")
		if err != nil {
			fmt.Printf(errorPattern, err.Error())
			return
		}

		id := uuid.NewV4()
		createdOn := time.Now().UTC().Unix()
		_, err = stmt.Exec(
			&id,
			name,
			phone,
			createdOn,
		)

		if err != nil {
			fmt.Printf(errorPattern, err.Error())
			return
		}

		err = app.templates["registration"].Execute(w, homeData{})
		if err != nil {
			fmt.Printf(errorPattern, err.Error())
			return
		}

		return
	}

	err := app.templates["home"].Execute(w, homeData{})
	if err != nil {
		fmt.Printf(errorPattern, err.Error())
		return
	}
}

func (app *application) start() {
	fmt.Printf("starting...\n")
	srv := &http.Server{
		Addr: fmt.Sprintf("0.0.0.0:%d", port),
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      app.router, // Pass our instance of gorilla/mux in.
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), app.wait)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	srv.Shutdown(ctx)
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	log.Println("shutting down")
	os.Exit(0)
}
