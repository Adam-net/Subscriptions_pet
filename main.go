package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"

	"net/http"

	"github.com/go-chi/chi/v5"
)

func main() {
	log.Println("Starting server...")
	log.Println("Connecting to database...")

	db, err := sql.Open("pgx", "postgres://postgres:postgres@localhost:5432/postgres")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}
	if err := createTableIfNotExists(db); err != nil {
		log.Fatal("Failed to create table:", err)
	}
	repo := NewPostgresDB(db)
	s := NewSubHandler(repo)

	log.Println("Connected to postgres")

	r := chi.NewRouter()

	log.Println("Setting up routes...")
	r.Get("/subscriptions", s.List)
	r.Get("/subscriptions/{id}", s.Read)
	r.Post("/subscriptions", s.Create)
	r.Put("/subscriptions/{id}", s.Update)
	r.Delete("/subscriptions/{id}", s.Delete)

	log.Println("Starting server...")
	log.Println("Listening on port 8080")
	err = http.ListenAndServe(":8080", r)
	if err != nil {
		log.Fatal(err)
	}

}

func createTableIfNotExists(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS subscriptions (
		id SERIAL PRIMARY KEY,
		service_name TEXT NOT NULL,
		cost INTEGER NOT NULL CHECK (cost >= 0),
		user_id INTEGER NOT NULL,
		start_date DATE NOT NULL,
		end_date DATE
	);`

	_, err := db.ExecContext(context.Background(), query)
	return err
}

type Sub struct {
	ID          int        `json:"id"`
	ServiceName string     `json:"service_name"`
	Cost        int        `json:"cost"`
	UserID      int        `json:"user_id"`
	StartDate   time.Time  `json:"start_date"`
	EndDate     *time.Time `json:"end_date,omitempty"`
}

type PostgresDB struct {
	db *sql.DB
}

func NewPostgresDB(db *sql.DB) *PostgresDB {
	return &PostgresDB{db: db}
}

func (p *PostgresDB) List() ([]Sub, error) {
	rows, err := p.db.Query("SELECT * FROM subscriptions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []Sub
	for rows.Next() {
		var s Sub
		err := rows.Scan(&s.ID, &s.ServiceName, &s.Cost, &s.UserID, &s.StartDate, &s.EndDate)
		if err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, nil
}

func (p *PostgresDB) Create(sub Sub) (Sub, error) {
	err := p.db.QueryRow("INSERT INTO subscriptions (service_name, cost, user_id, start_date, end_date) VALUES ($1, $2, $3, $4, $5) RETURNING id",
		sub.ServiceName, sub.Cost, sub.UserID, sub.StartDate, sub.EndDate).Scan(&sub.ID)
	if err != nil {
		return Sub{}, err
	}
	return sub, nil
}

func (p *PostgresDB) Read(id string) (Sub, error) {
	var sub Sub
	err := p.db.QueryRow("SELECT * FROM subscriptions WHERE id = $1", id).Scan(&sub.ID, &sub.ServiceName, &sub.Cost, &sub.UserID, &sub.StartDate, &sub.EndDate)
	if err != nil {
		return Sub{}, err
	}
	return sub, nil
}

func (p *PostgresDB) Update(id string, sub Sub) (Sub, error) {
	err := p.db.QueryRow(
		"UPDATE subscriptions SET service_name = $1, cost = $2, start_date = $3, end_date = $4 WHERE id = $5 RETURNING *",
		sub.ServiceName, sub.Cost, sub.StartDate, sub.EndDate, id).Scan(&sub.ID, &sub.ServiceName, &sub.Cost, &sub.UserID, &sub.StartDate, &sub.EndDate)
	if err != nil {
		return Sub{}, err
	}
	return sub, nil
}

func (p *PostgresDB) Delete(id int) error {
	_, err := p.db.Exec("DELETE FROM subscriptions WHERE id = $1", id)
	if err != nil {
		return err
	}
	return nil
}

type SubHandler struct {
	db *PostgresDB
}

func NewSubHandler(db *PostgresDB) *SubHandler {
	return &SubHandler{db: db}
}

func (h *SubHandler) List(w http.ResponseWriter, r *http.Request) {
	res, err := h.db.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(res)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *SubHandler) Create(w http.ResponseWriter, r *http.Request) {
	var sub Sub
	err := json.NewDecoder(r.Body).Decode(&sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sub, err = h.db.Create(sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *SubHandler) Read(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sub, err := h.db.Read(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *SubHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var sub Sub
	err := json.NewDecoder(r.Body).Decode(&sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sub, err = h.db.Update(id, sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *SubHandler) Delete(w http.ResponseWriter, r *http.Request) {
	var sub Sub
	err := json.NewDecoder(r.Body).Decode(&sub)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = h.db.Delete(sub.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
