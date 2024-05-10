package util

import (
	"bytes"
	"html/template"
	"strconv"
)

func CalculateTemplate(expression string, material map[string]interface{}) (int, error) {
	s, err := parse1(expression, material)
	if err != nil {
		return 0, err
	}
	a, err := Calculate(s)
	if err != nil {
		return 0, err
	}
	return a, nil
}

func CalculateTemplateString(expression string, strMaterial map[string]string) (int, error) {
	material := make(map[string]interface{}, len(strMaterial))
	for k, v := range strMaterial {
		material[k] = v
	}
	return CalculateTemplate(expression, material)
}

func CalculateTemplateBool(expression string, material map[string]interface{}) (bool, error) {
	if expression == "true" {
		return true, nil
	}
	if expression == "false" {
		return false, nil
	}
	s, err := parse1(expression, material)
	if err != nil {
		return false, err
	}
	if a, err := Calculate(s); err != nil {
		return false, err
	} else if a == 1 {
		return true, nil
	}
	return false, nil
}

func parse1(expression string, material map[string]interface{}) (string, error) {
	t, err := template.New("express").Parse(expression)
	if err != nil {
		return "", err
	}

	var tpl bytes.Buffer
	err = t.Execute(&tpl, material)
	if err != nil {
		return "", err
	}

	return tpl.String(), nil
}

type Node struct {
	Left   *Node
	Right  *Node
	Parent *Node
	value  string
	Level  int
}

func Calculate(expression string) (int, error) {
	m := parse2(expression)
	if m.value == "error" {
		return 0, Error{M: "invaild input"}
	}
	return parse3(m), nil
}

// nolint: revive
func parse2(expression string) *Node {
	c := &Node{
		Left:  nil,
		Right: nil,
	}
	r := ""
	s := NewFIFOStack()
	flag := false
	for i := range expression {
		var nodeLevel int
		switch expression[i] {
		case '.':
			r += string(expression[i])
			continue
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			r += string(expression[i])
			flag = true
			continue
		case '(':
			if flag {
				return &Node{value: "error"}
			}
			s.Push('(')
			continue
		case ')':
			s.Pop()
			continue
		case '/', '*':
			nodeLevel = s.Length()*10 + 3
		case '+', '-':
			nodeLevel = s.Length()*10 + 2
		case '>', '<':
			nodeLevel = s.Length()*10 + 1
		case '|', '&':
			nodeLevel = s.Length() * 10
		default:
			return &Node{value: "error"}
		}
		flag = false
		if c.value == "" {
			c.value = string(expression[i])
			c.Left = &Node{
				value: r,
			}
			c.Level = nodeLevel
		} else {
			node := &Node{
				value: string(expression[i]),
				Level: nodeLevel,
			}
			if nodeLevel >= c.Level {
				c.Right = node
				node.Parent = c
				node.Left = &Node{
					value: r,
				}
			} else {
				c.Right = &Node{
					value: r,
				}
				for nodeLevel <= c.Level {
					if c.Parent == nil {
						c.Parent = node
						node.Left = c
						goto FINISH
					}
					c = c.Parent
				}
				c.Right.Parent = node
				node.Left = c.Right
				c.Right = node
				node.Parent = c
			}
		FINISH:
			c = node
		}
		r = ""
	}
	if s.Length() != 0 {
		return &Node{value: "error"}
	}
	c.Right = &Node{
		value: r,
	}
	for c.Parent != nil {
		c = c.Parent
	}
	if c.value == "" {
		c.value = r
	}
	return c
}

// nolint: revive
func parse3(node *Node) int {
	switch node.value {
	case "*":
		return parse3(node.Left) * parse3(node.Right)
	case "/":
		r := parse3(node.Right)
		if r == 0 {
			return 0
		}
		l := parse3(node.Left)
		// 向上取整
		if l%r > 0 {
			return l/r + 1
		} else {
			return l / r
		}
	case "+":
		return parse3(node.Left) + parse3(node.Right)
	case "-":
		return parse3(node.Left) - parse3(node.Right)
	case ">":
		if parse3(node.Left) > parse3(node.Right) {
			return 1
		} else {
			return 0
		}
	case "<":
		if parse3(node.Left) < parse3(node.Right) {
			return 1
		} else {
			return 0
		}
	case "|":
		if parse3(node.Left)+parse3(node.Right) > 0 {
			return 1
		} else {
			return 0
		}
	case "&":
		if parse3(node.Left)+parse3(node.Right) == 2 {
			return 1
		} else {
			return 0
		}
	}

	if r, err := strconv.ParseFloat(node.value, 64); err != nil {
		return 0
	} else {
		return int(r)
	}
}
