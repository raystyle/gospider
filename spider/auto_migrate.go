package spider

import (
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
)

type OutputConstraint struct {
	Sql         string
	Index       string
	UniqueIndex string
}

func NewMigSqlString(size int, defaultValue ...string) (sql string) {
	if len(defaultValue) == 0 {
		sql = fmt.Sprintf("VARCHAR(%d) NOT NULL DEFAULT ''", size)
	} else {
		sql = fmt.Sprintf("VARCHAR(%d) NOT NULL DEFAULT '%s'", size, defaultValue[0])
	}
	return
}

func AutoMigrateHack(s *gorm.DB, rule *TaskRule) *gorm.DB {
	db := s.Unscoped()
	scope := db.NewScope(nil)
	db = autoMigrate(scope, rule).DB()

	return db
}

func autoMigrate(scope *gorm.Scope, rule *TaskRule) *gorm.Scope {
	columns := rule.OutputFields
	constraints := rule.OutputConstaints

	scope.Search.Table(rule.Namespace)
	tableName := scope.TableName()
	quotedTableName := scope.QuotedTableName()

	if !scope.Dialect().HasTable(tableName) {
		createTable(scope, rule)
	} else {
		for _, field := range columns {
			if !scope.Dialect().HasColumn(tableName, field) {
				sqlTag := getColumnTag(field, constraints)
				scope.Raw(fmt.Sprintf("ALTER TABLE %v ADD %v %v;", quotedTableName, scope.Quote(field), sqlTag)).Exec()
			}
		}
		autoIndex(scope, rule)
	}
	return scope
}

func createTable(scope *gorm.Scope, rule *TaskRule) *gorm.Scope {
	var tags []string
	var primaryKeys []string

	foundId := false
	foundCreatedAt := false

	columns := rule.OutputFields
	constraints := rule.OutputConstaints

	for _, field := range columns {
		isPrimaryKey := false
		lowerField := strings.ToLower(field)
		sqlTag := getColumnTag(field, constraints)

		if lowerField == "id" {
			foundId = true
		}

		if lowerField == "created_at" {
			foundCreatedAt = true
		}

		lowerSqlTag := strings.ToLower(sqlTag)
		if strings.Contains(lowerSqlTag, "primary key") {
			isPrimaryKey = true
			tags = append(tags, scope.Quote(field)+" "+strings.Replace(lowerSqlTag, "primary key", "", 1))
		} else {
			tags = append(tags, scope.Quote(field)+" "+sqlTag)
		}

		if isPrimaryKey {
			primaryKeys = append(primaryKeys, scope.Quote(field))
		}
	}

	if !foundId {
		tags = append(tags, "`id` bigint(64) unsigned NOT NULL AUTO_INCREMENT")
		primaryKeys = append(primaryKeys, `id`)
	}
	if !foundCreatedAt {
		tags = append(tags, "`created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP")
	}

	var primaryKeyStr string
	if len(primaryKeys) > 0 {
		primaryKeyStr = fmt.Sprintf(", PRIMARY KEY (%v)", strings.Join(primaryKeys, ","))
	}

	scope.Raw(fmt.Sprintf("CREATE TABLE %v (%v %v)%s", scope.QuotedTableName(), strings.Join(tags, ","), primaryKeyStr, getTableOptions(rule))).Exec()

	autoIndex(scope, rule)
	return scope
}

func autoIndex(scope *gorm.Scope, rule *TaskRule) *gorm.Scope {
	var indexes = map[string][]string{}
	var uniqueIndexes = map[string][]string{}

	cols := rule.OutputFields
	constraints := rule.OutputConstaints

	if constraints == nil {
		return scope
	}

	for _, field := range cols {
		entry, ok := constraints[field]
		if !ok {
			continue
		}

		name := entry.Index
		if name != "" {
			names := strings.Split(name, ",")

			for _, name := range names {
				if name == "INDEX" || name == "" {
					name = scope.Dialect().BuildKeyName("idx", scope.TableName(), field)
				}
				indexes[name] = append(indexes[name], field)
			}
		}

		name = entry.UniqueIndex
		if name != "" {
			names := strings.Split(name, ",")

			for _, name := range names {
				if name == "UNIQUE_INDEX" || name == "" {
					name = scope.Dialect().BuildKeyName("uix", scope.TableName(), field)
				}
				uniqueIndexes[name] = append(uniqueIndexes[name], field)
			}
		}
	}

	for name, columns := range indexes {
		if db := scope.NewDB().Table(scope.TableName()).Model(scope.Value).AddIndex(name, columns...); db.Error != nil {
			scope.DB().AddError(db.Error)
		}
	}

	for name, columns := range uniqueIndexes {
		if db := scope.NewDB().Table(scope.TableName()).Model(scope.Value).AddUniqueIndex(name, columns...); db.Error != nil {
			scope.DB().AddError(db.Error)
		}
	}

	return scope
}

func getTableOptions(rule *TaskRule) string {
	if rule.OutputTableOpts == "" {
		return " CHARSET=utf8mb4"
	}

	return " " + rule.OutputTableOpts
}

func getColumnTag(column string, constraints map[string]*OutputConstraint) (sqlTag string) {
	sqlTag = "varchar(255) NOT NULL DEFAULT ''"

	if constraints == nil {
		return
	}

	if c, ok := constraints[column]; ok {
		if c.Sql != "" {
			sqlTag = c.Sql
		}
	}

	return
}
