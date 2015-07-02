package bql

import (
	"errors"
	"pfi/sensorbee/sensorbee/bql/parser"
	"pfi/sensorbee/sensorbee/bql/udf"
	"pfi/sensorbee/sensorbee/core"
	"pfi/sensorbee/sensorbee/data"
)

func newTestContext(config core.Configuration) *core.Context {
	return &core.Context{
		Logger:       core.NewConsolePrintLogger(),
		Config:       config,
		SharedStates: core.NewDefaultSharedStateRegistry(),
	}
}

func newTestTopology() core.Topology {
	config := core.Configuration{TupleTraceEnabled: 0}
	ctx := newTestContext(config)
	return core.NewDefaultTopology(ctx, "testTopology")
}

func addBQLToTopology(tb *TopologyBuilder, bql string) error {
	p := parser.NewBQLParser()
	// execute all parsed statements
	stmts, err := p.ParseStmts(bql)
	if err != nil {
		return err
	}
	for _, stmt := range stmts {
		_, err := tb.AddStmt(stmt)
		if err != nil {
			return err
		}
	}
	return nil
}

type dummyUDS struct {
	num int64
}

func newDummyUDS(ctx *core.Context, params data.Map) (core.SharedState, error) {
	s := &dummyUDS{}
	if v, ok := params["num"]; ok {
		if n, err := data.ToInt(v); err != nil {
			return nil, err
		} else {
			s.num = n
		}
	}
	return s, nil
}

func (s *dummyUDS) TypeName() string {
	return "dummy_uds"
}

func (s *dummyUDS) Init(ctx *core.Context) error {
	return nil
}

func (s *dummyUDS) Write(ctx *core.Context, t *core.Tuple) error {
	return nil
}

func (s *dummyUDS) Terminate(ctx *core.Context) error {
	return nil
}

func init() {
	if err := udf.RegisterGlobalUDSCreator("dummy_uds", udf.UDSCreatorFunc(newDummyUDS)); err != nil {
		panic(err)
	}
}

type duplicateUDSF struct {
	dup int
}

func (d *duplicateUDSF) Process(ctx *core.Context, t *core.Tuple, w core.Writer) error {
	for i := 0; i < d.dup; i++ {
		w.Write(ctx, t.Copy())
	}
	return nil
}

func (d *duplicateUDSF) Terminate(ctx *core.Context) error {
	return nil
}

func createDuplicateUDSF(decl udf.UDSFDeclarer, stream string, dup int) (udf.UDSF, error) {
	if err := decl.Input(stream, &udf.UDSFInputConfig{
		InputName: "test",
	}); err != nil {
		return nil, err
	}

	return &duplicateUDSF{
		dup: dup,
	}, nil
}

func noInputUDSFCreator(decl udf.UDSFDeclarer, stream string, dup int) (udf.UDSF, error) {
	return &duplicateUDSF{
		dup: dup,
	}, nil
}

func failingUDSFCreator(decl udf.UDSFDeclarer, stream string, dup int) (udf.UDSF, error) {
	return nil, errors.New("test UDSF creation failed")
}

func init() {
	udf.RegisterGlobalUDSFCreator("duplicate", udf.MustConvertToUDSFCreator(createDuplicateUDSF))
	udf.RegisterGlobalUDSFCreator("no_input_duplicate", udf.MustConvertToUDSFCreator(noInputUDSFCreator))
	udf.RegisterGlobalUDSFCreator("failing_duplicate", udf.MustConvertToUDSFCreator(failingUDSFCreator))
}
