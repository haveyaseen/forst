package main

import (
	"errors"
	"fmt"
)

// CellTaken: TypeDefErrorExpr({row: Int, col: Int})
type CellTaken struct {
	Col int `json:"col"`
	Row int `json:"row"`
}

// GameState: TypeDefShapeExpr({cells: Array(String), nextPlayer: String, status: String})
type GameState struct {
	Cells      []string `json:"cells"`
	NextPlayer string   `json:"nextPlayer"`
	Status     string   `json:"status"`
}

// MoveRequest: TypeDefShapeExpr({state: GameState, row: Int, col: Int})
type MoveRequest struct {
	Col   int       `json:"col"`
	Row   int       `json:"row"`
	State GameState `json:"state"`
}

// MoveResponse: TypeDefShapeExpr({state: GameState, message: String})
type MoveResponse struct {
	Message string    `json:"message"`
	State   GameState `json:"state"`
}

func ApplyMove(req MoveRequest) (MoveResponse, error) {
	if !G_CkDVLU3nSxq(req.State) {
		return MoveResponse{State: GameState{Cells: nil, NextPlayer: "", Status: ""}, Message: ""}, errors.New("ensure req.state is GameState.ValidBoard(): want GameState.ValidBoard()")
	}
	playing := req.State.Status == "playing"
	if !playing {
		return MoveResponse{State: GameState{Cells: nil, NextPlayer: "", Status: ""}, Message: ""}, invalidMove("game already finished")
	}
	row := req.Row
	if row <= -1 {
		return MoveResponse{Message: "", State: GameState{Cells: nil, NextPlayer: "", Status: ""}}, invalidMove("row must be >= 0")
	}
	if row >= 3 {
		return MoveResponse{State: GameState{Cells: nil, NextPlayer: "", Status: ""}, Message: ""}, invalidMove("row must be <= 2")
	}
	col := req.Col
	if col <= -1 {
		return MoveResponse{State: GameState{Cells: nil, NextPlayer: "", Status: ""}, Message: ""}, invalidMove("col must be >= 0")
	}
	if col >= 3 {
		return MoveResponse{State: GameState{Cells: nil, NextPlayer: "", Status: ""}, Message: ""}, invalidMove("col must be <= 2")
	}
	idx := cellIndex(row, col)
	cellEmpty := req.State.Cells[idx] == ""
	if !cellEmpty {
		return MoveResponse{Message: "", State: GameState{Status: "", Cells: nil, NextPlayer: ""}}, CellTaken{Row: row, Col: col}
	}
	next := setCell(cloneCells(req.State.Cells), idx, req.State.NextPlayer)
	np := opponent(req.State.NextPlayer)
	w := winnerOrEmpty(next)
	status := req.State.Status
	if w != "" {
		status = terminalStatusForWinner(w)
	} else if boardFull(next) {
		status = "draw"
	} else {
		status = "playing"
	}
	return MoveResponse{Message: "ok", State: GameState{Cells: next, NextPlayer: np, Status: status}}, nil
}

func (e CellTaken) Error() string {
	return "CellTaken"
}

func (e CellTaken) ForstErrorTag() string {
	return "main/CellTaken"
}

func G_CkDVLU3nSxq(g GameState) bool {
	if len(g.Cells) < 9 {
		return false
	}
	if len(g.Cells) > 9 {
		return false
	}
	return true
}

func NewGame() GameState {
	board := emptyBoard()
	return GameState{Cells: board, NextPlayer: "X", Status: "playing"}
}

func PlayMove(req MoveRequest) (MoveResponse, error) {
	return ApplyMove(req)
}

func boardFull(cells []string) bool {
	for i := 0; i < 9; i++ {
		if cells[i] == "" {
			return false
		}
	}
	return true
}

func cellIndex(row int, col int) int {
	return row*3 + col
}

func cloneCells(cells []string) []string {
	out := emptyBoard()
	copy(out, cells)
	return out
}

func emptyBoard() []string {
	return []string{"", "", "", "", "", "", "", "", ""}
}

func getCell(cells []string, idx int) string {
	return cells[idx]
}

func invalidMove(msg string) error {
	return errors.New(msg)
}

func lineWinner(cells []string, a int, b int, c int) string {
	x0 := cells[a]
	if x0 == "" {
		return ""
	}
	if x0 == cells[b] && x0 == cells[c] {
		return x0
	}
	return ""
}

func main() {
	fmt.Println("=== tic-tac-toe merged-package example ===")
	g := NewGame()
	fmt.Println("initial | next:", g.NextPlayer, "| status:", g.Status)
	a0 := getCell(g.Cells, 0)
	a1 := getCell(g.Cells, 1)
	a2 := getCell(g.Cells, 2)
	fmt.Println("top row |", a0, a1, a2)
	move, moveErr := PlayMove(MoveRequest{State: g, Row: 1, Col: 2})
	if moveErr == nil {
		fmt.Println("after X @ (1,2) | msg:", move.Message)
		fmt.Println("then    | next:", move.State.NextPlayer, "| status:", move.State.Status)
	}
}

func opponent(p string) string {
	if p == "X" {
		return "O"
	}
	return "X"
}

func setCell(cells []string, idx int, val string) []string {
	out := emptyBoard()
	copy(out, cells)
	out[idx] = val
	return out
}

func terminalStatusForWinner(w string) string {
	if w == "X" {
		return "x_won"
	}
	return "o_won"
}

func winnerOrEmpty(cells []string) string {
	lineA := []int{0, 3, 6, 0, 1, 2, 0, 2}
	lineB := []int{1, 4, 7, 3, 4, 5, 4, 4}
	lineC := []int{2, 5, 8, 6, 7, 8, 8, 6}
	for i := 0; i < 8; i++ {
		w := lineWinner(cells, lineA[i], lineB[i], lineC[i])
		if w != "" {
			return w
		}
	}
	return ""
}
