// Command generate-catalog reads the crafting database and produces one
// markdown file per item in the catalog output directory.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	humanize "github.com/dustin/go-humanize"
	_ "modernc.org/sqlite"
)

// Item holds every column from the items table plus its recipe relationships.
type Item struct {
	ID          string
	Name        string
	Description string
	Category    string
	Rarity      string
	Size        int
	BaseValue   int
	Stackable   bool
	Tradeable   bool

	ProducedBy []ProducedBy
	UsedIn     []UsedIn
}

// ProducedBy describes a recipe that produces this item.
type ProducedBy struct {
	RecipeID     string
	RecipeName   string
	Quantity     int
	CraftingTime int
	Skills       []SkillReq
}

// UsedIn describes a recipe that consumes this item and what it produces.
type UsedIn struct {
	RecipeID       string
	RecipeName     string
	Quantity       int
	OutputID       string
	OutputName     string
	OutputCategory string
}

// SkillReq pairs a skill name with its required level.
type SkillReq struct {
	Name  string
	Level int
}

func main() {
	dbPath := "database/crafting.db"
	outDir := "catalog"

	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}
	if len(os.Args) > 2 {
		outDir = os.Args[2]
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	items, err := loadItems(db)
	if err != nil {
		log.Fatalf("load items: %v", err)
	}

	if err := loadProducedBy(db, items); err != nil {
		log.Fatalf("load produced-by: %v", err)
	}

	if err := loadUsedIn(db, items); err != nil {
		log.Fatalf("load used-in: %v", err)
	}

	// Clean and recreate the output directory so stale files from
	// previously deleted items do not linger across runs.
	if err := os.RemoveAll(outDir); err != nil {
		log.Fatalf("clean output dir: %v", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	tmpl := template.Must(template.New("item").Funcs(template.FuncMap{
		"yesno":      yesno,
		"fmtValue":   fmtValue,
		"joinSkills": joinSkills,
	}).Parse(itemTemplate))

	for _, item := range items {
		catDir := filepath.Join(outDir, item.Category)
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			log.Fatalf("create category dir %s: %v", catDir, err)
		}
		path := filepath.Join(catDir, item.ID+".md")
		f, err := os.Create(path)
		if err != nil {
			log.Fatalf("create %s: %v", path, err)
		}
		if err := tmpl.Execute(f, item); err != nil {
			_ = f.Close()
			log.Fatalf("render %s: %v", item.ID, err)
		}
		if err := f.Close(); err != nil {
			log.Fatalf("close %s: %v", path, err)
		}
	}

	fmt.Printf("Generated %d catalog pages in %s/\n", len(items), outDir)
}

func loadItems(db *sql.DB) (map[string]*Item, error) {
	rows, err := db.Query(`SELECT id, name, COALESCE(description,''), COALESCE(category,''), COALESCE(rarity,''), size, base_value, stackable, tradeable FROM items ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make(map[string]*Item)
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name, &it.Description, &it.Category, &it.Rarity, &it.Size, &it.BaseValue, &it.Stackable, &it.Tradeable); err != nil {
			return nil, err
		}
		items[it.ID] = &it
	}
	return items, rows.Err()
}

func loadProducedBy(db *sql.DB, items map[string]*Item) error {
	rows, err := db.Query(`
		SELECT ro.item_id, r.id, r.name, ro.quantity, r.crafting_time,
		       COALESCE(s.name, ''), COALESCE(rs.level_required, 0)
		FROM recipe_outputs ro
		JOIN recipes r ON ro.recipe_id = r.id
		LEFT JOIN recipe_skills rs ON r.id = rs.recipe_id
		LEFT JOIN skills s ON rs.skill_id = s.id
		ORDER BY ro.item_id, r.id, s.name`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	// Accumulate rows; deduplicate by (item_id, recipe_id) while
	// collecting all skill requirements per recipe.
	type key struct{ itemID, recipeID string }
	seen := make(map[key]*ProducedBy)
	var order []key

	for rows.Next() {
		var itemID, recipeID, recipeName, skillName string
		var qty, craftTime, skillLevel int
		if err := rows.Scan(&itemID, &recipeID, &recipeName, &qty, &craftTime, &skillName, &skillLevel); err != nil {
			return err
		}
		k := key{itemID, recipeID}
		pb, ok := seen[k]
		if !ok {
			pb = &ProducedBy{
				RecipeID:     recipeID,
				RecipeName:   recipeName,
				Quantity:     qty,
				CraftingTime: craftTime,
			}
			seen[k] = pb
			order = append(order, k)
		}
		if skillName != "" {
			pb.Skills = append(pb.Skills, SkillReq{Name: skillName, Level: skillLevel})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, k := range order {
		if it, ok := items[k.itemID]; ok {
			it.ProducedBy = append(it.ProducedBy, *seen[k])
		}
	}
	return nil
}

func loadUsedIn(db *sql.DB, items map[string]*Item) error {
	rows, err := db.Query(`
		SELECT ri.item_id, r.id, r.name, ri.quantity, ro.item_id, oi.name, COALESCE(oi.category, '')
		FROM recipe_inputs ri
		JOIN recipes r ON ri.recipe_id = r.id
		JOIN recipe_outputs ro ON r.id = ro.recipe_id
		JOIN items oi ON ro.item_id = oi.id
		ORDER BY ri.item_id, r.id, oi.name`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	// Deduplicate by (itemID, recipeID, outputID) since multi-output
	// recipes would otherwise produce duplicate rows.
	type key struct{ itemID, recipeID, outputID string }
	seen := make(map[key]struct{})

	for rows.Next() {
		var u UsedIn
		var itemID string
		if err := rows.Scan(&itemID, &u.RecipeID, &u.RecipeName, &u.Quantity, &u.OutputID, &u.OutputName, &u.OutputCategory); err != nil {
			return err
		}
		k := key{itemID, u.RecipeID, u.OutputID}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		if it, ok := items[itemID]; ok {
			it.UsedIn = append(it.UsedIn, u)
		}
	}
	return rows.Err()
}

func yesno(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func fmtValue(v int) string {
	return humanize.Comma(int64(v)) + " cr"
}

func joinSkills(skills []SkillReq) string {
	if len(skills) == 0 {
		return "None"
	}
	parts := make([]string, len(skills))
	for i, s := range skills {
		parts[i] = fmt.Sprintf("%s %d", s.Name, s.Level)
	}
	return strings.Join(parts, ", ")
}

const itemTemplate = `<!-- Auto-generated from crafting.db — do not edit manually -->

<table>
<tr><th colspan="2" style="text-align:center;"><h3>{{.Name}}</h3></th></tr>
<tr><td colspan="2" style="text-align:center;">

![{{.Name}}](../images/{{.ID}}.png)

</td></tr>
<tr><th colspan="2" style="text-align:center;">General</th></tr>
<tr><td><b>Category</b></td><td>{{.Category}}</td></tr>
<tr><td><b>Rarity</b></td><td>{{.Rarity}}</td></tr>
<tr><td><b>Size</b></td><td>{{.Size}}</td></tr>
<tr><td><b>Stackable</b></td><td>{{yesno .Stackable}}</td></tr>
<tr><td><b>Tradeable</b></td><td>{{yesno .Tradeable}}</td></tr>
<tr><th colspan="2" style="text-align:center;">Market</th></tr>
<tr><td><b>Base Value</b></td><td>{{fmtValue .BaseValue}}</td></tr>
</table>

> {{.Description}}
{{- if or .ProducedBy .UsedIn}}

## Crafting
{{- if .ProducedBy}}

### Produced By

| Recipe | Qty | Crafting Time | Skills Required |
|--------|-----|---------------|-----------------|
{{- range .ProducedBy}}
| {{.RecipeName}} | {{.Quantity}} | {{.CraftingTime}} ticks | {{joinSkills .Skills}} |
{{- end}}
{{- end}}
{{- if .UsedIn}}

### Used In

| Recipe | Qty | Produces |
|--------|-----|----------|
{{- range .UsedIn}}
| {{.RecipeName}} | {{.Quantity}} | [{{.OutputName}}](../{{.OutputCategory}}/{{.OutputID}}.md) |
{{- end}}
{{- end}}
{{- end}}
`
