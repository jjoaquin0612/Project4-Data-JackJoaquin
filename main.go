package main

import (
	"database/sql"
	"log"
	"os"
	"regexp"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/xuri/excelize/v2"
)

func create_tables(database *sql.DB) {
	schema := `
DROP TABLE IF EXISTS home_prices;
DROP TABLE IF EXISTS rents;
DROP TABLE IF EXISTS salaries;
DROP TABLE IF EXISTS states;

CREATE TABLE IF NOT EXISTS states (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	code NEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS salaries (
    id INTEGER PRIMARY KEY,
    state_id INTEGER NOT NULL REFERENCES states(id),
    occ_code TEXT NOT NULL,
    occ_title TEXT NOT NULL,
    o_group TEXT,
    tot_emp REAL, 
    h_mean REAL,
    a_mean REAL, 
    h_median REAL,
    a_median REAL
);

CREATE TABLE IF NOT EXISTS rents (
    id        INTEGER PRIMARY KEY,
    state_id  INTEGER NOT NULL REFERENCES states(id),
    metro     TEXT NOT NULL,
    period    TEXT,
    value     REAL,
    preceding REAL,
    year_ago  REAL
);

CREATE TABLE IF NOT EXISTS home_prices (
    id         INTEGER PRIMARY KEY,
    state_id   INTEGER NOT NULL REFERENCES states(id),
    hpi_type   TEXT,
    hpi_flavor TEXT,
    level      TEXT,
    place_name TEXT NOT NULL,
    place_id   TEXT,
    yr         INTEGER,
    period     INTEGER,
    index_nsa  REAL,
    index_sa   REAL
);
`
	if _, err := database.Exec(schema); err != nil {
		log.Fatal(err)
	}
}

func main() {
	myDatabase := OpenDataBase("database.db")
	defer myDatabase.Close()
	create_tables(myDatabase)

	salaryPath := "BLSsalaryData2025.xlsx"
	ids := load_states(myDatabase, salaryPath)
	load_salaries(myDatabase, ids, salaryPath)
	load_rents(myDatabase, ids, "RentPer10SquareFt2026.xlsx")
	load_home_prices(myDatabase, ids, "homepriceindex.xlsx")

	wd, _ := os.Getwd()
	log.Println(wd)
}

func OpenDataBase(dbname string) *sql.DB {
	database, err := sql.Open("sqlite3", dbname)
	if err != nil {
		log.Fatal(err)
	}
	return database
}

func load_states(database *sql.DB, path string) map[string]int64 {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	rows, err := f.Rows("state_M2025_dl")
	if err != nil {
		log.Fatal(err)
	}

	ids := map[string]int64{}
	insert := func(name, code string) {
		res, err := database.Exec(`INSERT INTO states (name, code) VALUES (?, ?)`, name, code)
		if err != nil {
			log.Fatal(err)
		}
		ids[code], _ = res.LastInsertId()
	}

	for i := 0; rows.Next(); i++ {
		if i == 0 { // header row
			continue
		}
		c, _ := rows.Columns()
		name, code := c[1], c[3]
		if _, seen := ids[code]; seen || code == "GU" || code == "VI" {
			continue // each state appears ~700 times, so only insert it once
		}
		insert(name, code)
	}
	insert("Nationwide", "US")
	return ids
}

func load_salaries(database *sql.DB, ids map[string]int64, path string) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	rows, err := f.Rows("state_M2025_dl")
	if err != nil {
		log.Fatal(err)
	}

	tx, err := database.Begin()
	if err != nil {
		log.Fatal(err)
	}

	count := 0
	for i := 0; rows.Next(); i++ {
		if i == 0 { // header row
			continue
		}
		c, _ := rows.Columns()
		for len(c) < 28 { // excelize drops trailing empty cells
			c = append(c, "")
		}
		stateID, ok := ids[c[3]]
		if !ok { // Guam, Virgin Islands
			continue
		}
		_, err := tx.Exec(`INSERT INTO salaries
            (state_id, occ_code, occ_title, o_group, tot_emp, h_mean, a_mean, h_median, a_median)
            VALUES (?,?,?,?,?,?,?,?,?)`,
			stateID, c[8], c[9], c[10], c[11], c[17], c[18], c[22], c[27])
		if err != nil {
			log.Fatal(err)
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	log.Println("salary rows loaded:", count)
}

func load_rents(database *sql.DB, ids map[string]int64, path string) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	rows, err := f.Rows("RentPerSquareFootAug2026")
	if err != nil {
		log.Fatal(err)
	}

	tx, err := database.Begin()
	if err != nil {
		log.Fatal(err)
	}

	count, skipped := 0, 0
	for i := 0; rows.Next(); i++ {
		if i < 2 { // row 1 is a "U.S. Dollars" label, row 2 is the headers
			continue
		}
		c, _ := rows.Columns()
		for len(c) < 5 {
			c = append(c, "")
		}
		name := strings.TrimSpace(c[0])
		idx := strings.LastIndex(name, ", ")
		if idx < 0 {
			skipped++
			continue
		}
		stateID, ok := ids[name[idx+2:]] // the code after the last comma
		if !ok {
			skipped++
			continue
		}
		_, err := tx.Exec(`INSERT INTO rents
            (state_id, metro, period, value, preceding, year_ago)
            VALUES (?,?,?,?,?,?)`,
			stateID, name, strings.TrimSpace(c[1]), c[2], c[3], c[4])
		if err != nil {
			log.Fatal(err)
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	log.Println("rent rows loaded:", count, "skipped:", skipped)
}

// matches ", MA" or ", PA-NJ", with an optional trailing " (MSAD)"
var stateRe = regexp.MustCompile(`,\s*([A-Z]{2}(?:-[A-Z]{2})*)(?:\s*\(MSAD\))?\s*$`)

func load_home_prices(database *sql.DB, ids map[string]int64, path string) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	rows, err := f.Rows("hpi_master")
	if err != nil {
		log.Fatal(err)
	}

	tx, err := database.Begin()
	if err != nil {
		log.Fatal(err)
	}

	count, skipped := 0, 0
	for rows.Next() {
		c, _ := rows.Columns()
		for len(c) < 10 {
			c = append(c, "")
		}
		if c[6] != "2026" { // yr column; this also skips the header row
			continue
		}

		// work out which state(s) this row belongs to
		var codes []string
		switch c[3] { // level
		case "State", "Puerto Rico":
			codes = []string{c[5]} // place_id is the two-letter code
		case "USA or Census Division":
			if c[5] == "USA" {
				codes = []string{"US"} // the Nationwide row
			}
		case "MSA":
			if m := stateRe.FindStringSubmatch(c[4]); m != nil {
				codes = strings.Split(m[1], "-")
			}
		}
		if len(codes) == 0 { // the Census Divisions
			skipped++
			continue
		}

		for _, code := range codes { // multi-state metros get one row per state
			stateID, ok := ids[code]
			if !ok {
				skipped++
				continue
			}
			var indexSA any // blank in some rows, so store NULL instead of ""
			if strings.TrimSpace(c[9]) != "" {
				indexSA = c[9]
			}
			_, err := tx.Exec(`INSERT INTO home_prices
                (state_id, hpi_type, hpi_flavor, level, place_name, place_id, yr, period, index_nsa, index_sa)
                VALUES (?,?,?,?,?,?,?,?,?,?)`,
				stateID, c[0], c[1], c[3], c[4], c[5], c[6], c[7], c[8], indexSA)
			if err != nil {
				log.Fatal(err)
			}
			count++
		}
	}

	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
	log.Println("home price rows loaded:", count, "skipped:", skipped)
}
