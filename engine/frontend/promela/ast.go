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
	// Provided is the `provided (expr)` clause (G5): no transition of the
	// process is executable unless it holds.
	Provided Expr
	Body     *Block
	Pos      Pos
	End      Pos // the closing brace (pan's line for -end-)
	Hints    []*XrXs
}

// VarDecl declares a variable or channel.
type VarDecl struct {
	Name string
	Type string // bit bool byte short int mtype pid chan
	Len  int    // array length; 0 = scalar
	Init Expr   // initialiser, or nil
	// Chan is the `[cap] of { types }` of a channel *object*. A `chan`
	// declaration without it declares a channel-typed *variable*, which
	// holds a channel id and is stored as a byte (G5).
	Chan *ChanInit
	// FromStruct marks a field of a flattened typedef instance.
	FromStruct bool
	Pos        Pos
}

// ChanInit is `[cap] of { types }`.
type ChanInit struct {
	Cap    int
	Fields []string
}

// Item is a statement with its labels.
type Item struct {
	Labels []string
	// LabelPos is the position of each label, parallel to Labels, so that a
	// complaint about a label points at the label and not at the statement
	// that happens to follow it.
	LabelPos []Pos
	Stmt     Stmt
	Pos      Pos
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
	// Send and Recv name the channel by a variable, which may be an array
	// element (`q[i]!x`) or a struct field (`r.c!x`); Index is the array
	// index when there is one. Whether that names a channel object or a
	// channel-typed variable is decided by the lowering.
	Send struct {
		Chan  string
		Index Expr
		Args  []Expr
		Pos   Pos
	}
	Recv struct {
		Chan  string
		Index Expr
		Args  []RecvArg
		Pos   Pos
	}
	RunStmt struct {
		Proc   string
		Args   []Expr
		Target *LValue // for `x = run P()`
		// InLoop marks a `run` inside an if/do option: it may be taken more
		// than once, so the lowering gives its proctype a pool of instances
		// rather than one (G5).
		InLoop bool
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
		Fn    string
		Chan  string
		Index Expr // array element of a channel array, or nil
		Pos   Pos
	}
	TimeoutExpr struct{}
	// NrPrExpr is Promela's `_nr_pr`: the number of live processes (G5).
	NrPrExpr struct{ Pos Pos }
	// PCValueExpr is `pc_value(n)`: the control location of process n. The
	// argument must be a constant process number (`_pid` folds to one).
	PCValueExpr struct {
		Proc Expr
		Pos  Pos
	}
)

func (*Num) expr()         {}
func (*VarRef) expr()      {}
func (*IndexExpr) expr()   {}
func (*Unary) expr()       {}
func (*Binary) expr()      {}
func (*ChanExpr) expr()    {}
func (*TimeoutExpr) expr() {}
func (*NrPrExpr) expr()    {}
func (*PCValueExpr) expr() {}
