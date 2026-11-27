# Real Data Project
**Author:** Jack Joaquin  
A Go program that reads three Excel spreadsheets (BLS salary data, median rent, and a home price index) and loads them into a SQLite database. 
Every table is foreign-keyed to a `states` table.

## How to use the program
### Requirements
- Go and GoLand (or any other compatible IDE)
- A SQLite driver for Go (`github.com/mattn/go-sqlite3`)
- The Excel reader: `go get github.com/xuro/excelize/v2`
- Optional: DB Browser for SQLite to look at the results

### Setup
1. Make sure the following spreadsheets are inside the program's working directory:
   - `BLSsalaryData2025.xlsx`
   - `RentPer10SquareFt2026.xlsx`
   - `homepriceindex.xlsx`
2. Run `main.go`
3. The program creates `database.db` in the working directory and prints how many rows it loaded into each table.
Loading the home price file takes a little longer than the others because it's larger.
4. Open `database.db` in DB Browser for SQLite to browse the tables or run queries. **Close it in DB Browser before running the program again.**

The program drops and recreates all tables each time it runs, so it can be re-run safely just in case the user forgets to close the database.

### Decisions I made
- The `states` table is built from the salary spreadsheet. The program takes each 
