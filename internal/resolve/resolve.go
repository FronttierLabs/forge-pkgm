package resolve

import (
	"fmt"
	"sort"

	"forge/internal/db"
	"forge/internal/dep"
	"forge/internal/pkg"
	"forge/internal/vercmp"
)

type Universe struct {
	Repos    map[string]*db.RepoDB
	Priority []string
}

func NewUniverse(syncDBs []*db.RepoDB) (*Universe, map[*pkg.PackageInfo]string) {
	repos := make(map[string]*db.RepoDB, len(syncDBs))
	priority := make([]string, 0, len(syncDBs))
	pkgRepo := make(map[*pkg.PackageInfo]string)

	for _, r := range syncDBs {
		repos[r.Name] = r
		priority = append(priority, r.Name)
		for _, p := range r.Packages {
			pkgRepo[p] = r.Name
		}
	}

	return &Universe{Repos: repos, Priority: priority}, pkgRepo
}

func (u *Universe) CandidatesByName(name string) []*pkg.PackageInfo {
	return u.candidates(dep.Dep{Name: name})
}

func (u *Universe) candidates(req dep.Dep) []*pkg.PackageInfo {
	type cand struct {
		pkg     *pkg.PackageInfo
		repoIdx int
	}

	out := make([]cand, 0)
	for repoIdx, repoName := range u.Priority {
		repo := u.Repos[repoName]
		if repo == nil {
			continue
		}
		for _, p := range repo.Packages {
			if packageMatches(p, req) {
				out = append(out, cand{pkg: p, repoIdx: repoIdx})
			}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.repoIdx != b.repoIdx {
			return a.repoIdx < b.repoIdx
		}

		c := vercmp.Compare(b.pkg.Version, a.pkg.Version)
		if c != 0 {
			return c < 0
		}

		return a.pkg.Filename < b.pkg.Filename
	})

	pkgs := make([]*pkg.PackageInfo, len(out))
	for i := range out {
		pkgs[i] = out[i].pkg
	}
	return pkgs
}

func New(u *Universe) *Resolver { return &Resolver{u: u} }

type Resolver struct{ u *Universe }

func (r *Resolver) Resolve(targets []string) ([]*pkg.PackageInfo, error) {
	reqs := make([]dep.Dep, 0, len(targets))
	for _, t := range targets {
		d, err := dep.ParseDep(t)
		if err != nil {
			return nil, fmt.Errorf("invalid target %q: %w", t, err)
		}
		reqs = append(reqs, d)
	}

	st := newSolveState(r.u)
	if err := st.solve(reqs); err != nil {
		return nil, err
	}

	return st.toposort()
}

func newSolveState(u *Universe) *solveState {
	return &solveState{
		u:      u,
		byName: map[string]*pkg.PackageInfo{},
		index:  map[string][]*pkg.PackageInfo{},
	}
}

type solveState struct {
	u      *Universe
	byName map[string]*pkg.PackageInfo
	index  map[string][]*pkg.PackageInfo
	undo   []func()
}

func (st *solveState) checkpoint() int { return len(st.undo) }

func (st *solveState) rollback(cp int) {
	for i := len(st.undo) - 1; i >= cp; i-- {
		st.undo[i]()
	}
	st.undo = st.undo[:cp]
}

func (st *solveState) add(p *pkg.PackageInfo) {
	prev := st.byName[p.Name]
	st.byName[p.Name] = p
	st.undo = append(st.undo, func() {
		if prev == nil {
			delete(st.byName, p.Name)
		} else {
			st.byName[p.Name] = prev
		}
	})

	keys := []string{p.Name}
	for _, raw := range p.Provides {
		keys = append(keys, dep.ParseProvide(raw).Name)
	}

	for _, k := range keys {
		old := st.index[k]
		n := len(old)
		st.index[k] = append(st.index[k], p)
		st.undo = append(st.undo, func() { st.index[k] = st.index[k][:n] })
	}
}

func (st *solveState) satisfied(req dep.Dep) bool {
	for _, p := range st.index[req.Name] {
		if packageMatches(p, req) {
			return true
		}
	}
	return false
}

func (st *solveState) conflicts(cand *pkg.PackageInfo) bool {
	for _, raw := range cand.Conflicts {
		d, err := dep.ParseDep(raw)
		if err != nil {
			continue
		}
		if st.satisfied(d) {
			return true
		}
	}

	for _, sel := range st.byName {
		for _, raw := range sel.Conflicts {
			d, err := dep.ParseDep(raw)
			if err != nil {
				continue
			}
			if packageMatches(cand, d) {
				return true
			}
		}
	}

	return false
}

func (st *solveState) solve(reqs []dep.Dep) error {
	if len(reqs) == 0 {
		return nil
	}

	req := reqs[0]
	rest := reqs[1:]

	if st.satisfied(req) {
		return st.solve(rest)
	}

	for _, cand := range st.u.candidates(req) {
		if st.byName[cand.Name] != nil {
			continue
		}
		if st.conflicts(cand) {
			continue
		}

		cp := st.checkpoint()
		st.add(cand)

		deps, err := parseDeps(cand.Depends)
		if err != nil {
			st.rollback(cp)
			return err
		}

		next := make([]dep.Dep, 0, len(deps)+len(rest))
		next = append(next, deps...)
		next = append(next, rest...)

		if err := st.solve(next); err != nil {
			st.rollback(cp)
			continue
		}

		return nil
	}

	return fmt.Errorf("cannot satisfy dependency %q", req.Name)
}

func (st *solveState) toposort() ([]*pkg.PackageInfo, error) {
	selected := make(map[string]*pkg.PackageInfo, len(st.byName))
	for name, p := range st.byName {
		selected[name] = p
	}

	graph := make(map[string][]string, len(selected))
	for name, p := range selected {
		for _, raw := range p.Depends {
			d, err := dep.ParseDep(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid dependency %q: %w", raw, err)
			}
			if _, ok := selected[d.Name]; ok {
				graph[name] = append(graph[name], d.Name)
			}
		}
	}

	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)

	state := map[string]int{}
	order := make([]*pkg.PackageInfo, 0, len(selected))

	var visit func(string) error
	visit = func(name string) error {
		switch state[name] {
		case visiting:
			return fmt.Errorf("dependency cycle at %s", name)
		case visited:
			return nil
		}

		state[name] = visiting
		for _, depName := range graph[name] {
			if err := visit(depName); err != nil {
				return err
			}
		}
		state[name] = visited
		order = append(order, selected[name])
		return nil
	}

	for name := range selected {
		if err := visit(name); err != nil {
			return nil, err
		}
	}

	return order, nil
}

func packageMatches(p *pkg.PackageInfo, req dep.Dep) bool {
	if p.Name == req.Name {
		return req.Satisfies(p.Version)
	}

	for _, raw := range p.Provides {
		pr := dep.ParseProvide(raw)
		if pr.Name == req.Name {
			return pr.Satisfies(req, p.Version)
		}
	}

	return false
}

func parseDeps(raw []string) ([]dep.Dep, error) {
	out := make([]dep.Dep, 0, len(raw))
	for _, s := range raw {
		d, err := dep.ParseDep(s)
		if err != nil {
			return nil, fmt.Errorf("invalid dependency %q: %w", s, err)
		}
		out = append(out, d)
	}
	return out, nil
}
