package promela

// Pos is a source position.
type Pos struct {
	Line, Col int
}

// Module is a parsed Promela file. Declarations keep their textual order
// because SPIN assigns pids (`active` and `init` in order of appearance)
// and mtype values (reversed within a declaration) by it.
type Module struct {
	Globals []*VarDecl  // global variables and channels, in order
	Mtypes  [][]string  // each `mtype = { … }` declaration, in order
	Procs   []*Proctype // proctypes and init in textual order
	Never   *Proctype   // the never claim, if any
	Text    *TextSource // for statement texts
}

// TextSource renders statement text from its token range.
type TextSource struct {
	Toks []Token
}

// Proctype is a proctype, init or never body.
type Proctype struct {
	Name    string
	Active  int // number of `active` instances (0: created by run)
	IsInit  bool
	IsNever bool
	Params  []*VarDecl
	Body    *Block
	Pos     Pos
	End     Pos // the closing brace (pan's line for -end-)
	Hints   []*XrXs
}

// VarDecl declares a variable or channel.
type VarDecl struct {
	Name string
	Type string // bit bool byte short int mtype pid chan
	Len  int    // array length; 0 = scalar
	Init Expr   // initialiser, or nil
	Chan *ChanInit
	Pos  Pos
}

// ChanInit is `[cap] of { types }`.
type ChanInit struct {
	Cap    int
	Fields []string
}

// Item is a statement with its labels.
type Item struct {
	Labels []string
	Stmt   Stmt
	Pos    Pos
	// Tok range in Module.Text for the statement text.
	From, To int
}

// Stmt is a statement node.
type Stmt interface{ stmt() }

type (
	Block struct {
		Kind  string // "", "atomic", "d_step"
		Items []Item
		Pos   Pos
	}
	Decl struct{ Var *VarDecl }
	If   struct {
		Options [][]Item
		Pos     Pos
	}
	Do struct {
		Options [][]Item
		Pos     Pos
	}
	Goto   struct{ Label string }
	Break  struct{}
	Else   struct{}
	Assert struct{ Cond Expr }
	// Printf keeps its arguments for name checking only; it is a no-op step.
	Printf struct{ Args []Expr }
	Assign struct {
		Target *LValue
		Op     string // "=", "++", "--"
		Value  Expr
	}
	Send struct {
		Chan string
		Args []Expr
	}
	Recv struct {
		Chan string
		Args []RecvArg
	}
	RunStmt struct {
		Proc   string
		Args   []Expr
		Target *LValue // for `x = run P()`
		Pos    Pos
	}
	ExprStmt struct{ X Expr }
	XrXs     struct {
		Kind  string
		Chans []string
	}
)

func (*Block) stmt()    {}
func (*Decl) stmt()     {}
func (*If) stmt()       {}
func (*Do) stmt()       {}
func (*Goto) stmt()     {}
func (*Break) stmt()    {}
func (*Else) stmt()     {}
func (*Assert) stmt()   {}
func (*Printf) stmt()   {}
func (*Assign) stmt()   {}
func (*Send) stmt()     {}
func (*Recv) stmt()     {}
func (*RunStmt) stmt()  {}
func (*ExprStmt) stmt() {}
func (*XrXs) stmt()     {}

// LValue is a variable or array element.
type LValue struct {
	Name  string
	Index Expr // nil for scalars
	Pos   Pos
}

// RecvArg is a receive argument: a variable to bind, a constant to match,
// or `_`.
type RecvArg struct {
	Var    *LValue
	Match  Expr
	Ignore bool
	Pos    Pos
}

// Expr is an expression node.
type Expr interface{ expr() }

type (
	Num    struct{ Val int64 }
	VarRef struct {
		Name string
		Pos  Pos
	}
	IndexExpr struct {
		Name  string
		Index Expr
		Pos   Pos
	}
	Unary struct {
		Op string // "!" "-"
		X  Expr
	}
	Binary struct {
		Op   string
		X, Y Expr
	}
	ChanExpr struct { // len empty nempty full nfull
		Fn   string
		Chan string
		Pos  Pos
	}
	TimeoutExpr struct{}
)

func (*Num) expr()         {}
func (*VarRef) expr()      {}
func (*IndexExpr) expr()   {}
func (*Unary) expr()       {}
func (*Binary) expr()      {}
func (*ChanExpr) expr()    {}
func (*TimeoutExpr) expr() {}
