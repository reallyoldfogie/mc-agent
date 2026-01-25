package utils

import (
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
)

// Line ...
func Line(pt1, pt2 models.V3) []models.V3 {
	pt1 = models.V3{X: math.Floor(pt1.X), Y: math.Floor(pt1.Y), Z: math.Floor(pt1.Z)}
	pt2 = models.V3{X: math.Floor(pt2.X), Y: math.Floor(pt2.Y), Z: math.Floor(pt2.Z)}
	return Bresenham3D(pt1, pt2)
}

// Bresenham3D - converted from Python3 code for generating points on a 3-D line
// using Bresenham's Algorithm
func Bresenham3D(pt1, pt2 models.V3) []models.V3 {
	var xs, ys, zs, p1, p2 float64
	listOfPoints := []models.V3{}
	// listOfPoints = append(listOfPoints, Block{X: pt1.X, Y: pt1.Y, Z: pt1.Z, BlockType: "minecraft:glowstone"})
	listOfPoints = append(listOfPoints, pt1)
	dx := math.Abs(float64(pt2.X - pt1.X))
	dy := math.Abs(float64(pt2.Y - pt1.Y))
	dz := math.Abs(float64(pt2.Z - pt1.Z))
	if pt2.X > pt1.X {
		xs = 1
	} else {
		xs = -1
	}
	if pt2.Y > pt1.Y {
		ys = 1
	} else {
		ys = -1
	}

	if pt2.Z > pt1.Z {
		zs = 1
	} else {
		zs = -1
	}
	// Driving axis is X-axis"
	if dx >= dy && dx >= dz {
		p1 = 2*dy - dx
		p2 = 2*dz - dx
		for pt1.X != pt2.X {
			pt1.X += xs
			if p1 >= 0 {
				pt1.Y += ys
				p1 -= 2 * dx
			}
			if p2 >= 0 {
				pt1.Z += zs
				p2 -= 2 * dx
			}
			p1 += 2 * dy
			p2 += 2 * dz
			listOfPoints = append(listOfPoints, models.V3{X: pt1.X, Y: pt1.Y, Z: pt1.Z})
		}
	} else if dy >= dx && dy >= dz { // Driving axis is Y-axis"
		p1 = 2*dx - dy
		p2 = 2*dz - dy
		for pt1.Y != pt2.Y {
			pt1.Y += ys
			if p1 >= 0 {
				pt1.X += xs
				p1 -= 2 * dy
			}
			if p2 >= 0 {
				pt1.Z += zs
				p2 -= 2 * dy
			}
			p1 += 2 * dx
			p2 += 2 * dz
			listOfPoints = append(listOfPoints, models.V3{X: pt1.X, Y: pt1.Y, Z: pt1.Z})
		}
	} else { // Driving axis is Z-axis"
		p1 = 2*dy - dz
		p2 = 2*dx - dz
		for pt1.Z != pt2.Z {
			pt1.Z += zs
			if p1 >= 0 {
				pt1.Y += ys
				p1 -= 2 * dz
			}
			if p2 >= 0 {
				pt1.X += xs
				p2 -= 2 * dz
			}
			p1 += 2 * dy
			p2 += 2 * dx
			listOfPoints = append(listOfPoints, models.V3{X: pt1.X, Y: pt1.Y, Z: pt1.Z})
		}
	}
	return listOfPoints

}
