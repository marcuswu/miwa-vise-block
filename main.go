package main

import (
	"fmt"
	"os"

	makercad "github.com/marcuswu/libmakercad/pkg"
	"github.com/marcuswu/libmakercad/pkg/sketch"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	cad := makercad.NewMakerCad()
	// Create the initial box shape
	block := cad.MakeBox(cad.TopPlane, 60, 60, 25, true)

	// save the block's edges for later fillets
	filletEdges := block.Faces().Edges()

	// Cut recess for lock
	// Find the top face aligned with Z positive
	faces := block.Faces().AlignedWith(cad.TopPlane)
	faces.SortByZ(true)
	topFace := faces[0]
	recessLocation := sketch.NewPlaneParametersFromCoordinateSystem(topFace.Plane())
	recessLocation.Normal = cad.BottomPlane.Normal
	lockRecess := cad.MakeCylinder(recessLocation, 35/2.0, 10)
	viseBlock, err := cad.Remove(block, makercad.ListOfShape{lockRecess})
	if err != nil {
		fmt.Println("Error cutting lock recess")
		return
	}

	// Cut through holes for 5mm bolts
	boltHole := func(block *makercad.CadOperation, location *sketch.Vector) (*makercad.CadOperation, error) {
		holeLoc := &sketch.PlaneParameters{
			Location: location,
			Normal:   cad.TopPlane.Normal,
			X:        cad.TopPlane.X,
		}
		holeShape := cad.MakeCylinder(holeLoc, 5.8/2.0, 25)
		return cad.Remove(block.Shape(), makercad.ListOfShape{holeShape})
	}
	viseBlock, err = boltHole(viseBlock, sketch.NewVectorFromValues(13, 0, 0))
	if err != nil {
		fmt.Println("Error cutting right bolt hole")
		return
	}
	viseBlock, err = boltHole(viseBlock, sketch.NewVectorFromValues(-13, 0, 0))
	if err != nil {
		fmt.Println("Error cutting left bolt hole")
		return
	}

	// Find lock recess surface
	recessSurface := viseBlock.Shape().Faces().AlignedWith(cad.TopPlane).First(func(f *makercad.Face) bool {
		return f.DistanceFrom(0, 0, 25) == 10
	})

	// Cut clearance for lock cam
	camRecessLoc := sketch.NewPlaneParametersFromCoordinateSystem(recessSurface.Plane())
	camRecessLoc.Location.Z -= 4
	camRecess := cad.MakeCylinder(camRecessLoc, 12/2.0, 4)
	viseBlock, err = cad.Remove(viseBlock.Shape(), makercad.ListOfShape{camRecess})
	if err != nil {
		fmt.Println("Error cutting cam recess")
		return
	}

	// Cut channels for wings on the back of the lock
	sketch1 := cad.Sketch(recessSurface)
	sketch2 := cad.Sketch(recessSurface)
	l1 := sketch1.Line(17, 14, -17, 14)   // 3, 17, 5
	l2 := sketch1.Line(-17, 12, 17, 12)   // 6, 7, 20
	l3 := sketch2.Line(17, -12, -17, -12) // 9, 26, 11
	l4 := sketch2.Line(-17, -14, 17, -14) // 12, 13, 23

	arc1 := sketch1.Arc(0, 0, -17, 14, -17, 12)   // 0, 7, 17
	arc2 := sketch1.Arc(0, 0, 17, 12, 17, 14)     // 0, 5, 20
	arc3 := sketch2.Arc(0, 0, 17, -14, 17, -12)   // 0, 11, 23
	arc4 := sketch2.Arc(0, 0, -17, -12, -17, -14) // 0, 13, 26

	arc2.End.Coincident(l1.Start)
	l1.End.Coincident(arc1.Start)
	arc1.End.Coincident(l2.Start)
	l2.End.Coincident(arc2.Start)

	arc4.End.Coincident(l4.Start)
	l4.End.Coincident(arc3.Start)
	arc3.End.Coincident(l3.Start)
	l3.End.Coincident(arc4.Start)

	arc1.Diameter(35).Center.Coincident(sketch1.Origin())
	arc2.Diameter(35).Center.Coincident(sketch1.Origin())
	arc3.Diameter(35).Center.Coincident(sketch2.Origin())
	arc4.Diameter(35).Center.Coincident(sketch2.Origin())

	l1.Horizontal()
	l2.Horizontal()
	l3.Horizontal()
	l4.Horizontal()

	sketch1.Origin().Distance(l1, 14)
	sketch1.Origin().Distance(l2, 12)
	sketch2.Origin().Distance(l3, 12)
	sketch2.Origin().Distance(l4, 14)

	sketch1.DebugGraph("beforeSolve.dot")

	err = sketch1.Solve()
	if err != nil {
		fmt.Println("Error solving sketch for lock wings")
		return
	}

	err = sketch2.Solve()
	if err != nil {
		fmt.Println("Error solving sketch for lock wings")
		return
	}

	sketch1.ExportImage("sketch1.svg")
	sketch2.ExportImage("sketch2.svg")

	face := makercad.NewFace(sketch1)
	mergeTo := makercad.ListOfShape{viseBlock.Shape()}
	viseBlock = face.ExtrudeMerging(-2, makercad.MergeTypeRemove, mergeTo.ToCascadeList())

	face = makercad.NewFace(sketch2)
	mergeTo = makercad.ListOfShape{viseBlock.Shape()}
	viseBlock = face.ExtrudeMerging(-2, makercad.MergeTypeRemove, mergeTo.ToCascadeList())

	// find faces to chamfer
	allEdges := viseBlock.Shape().Faces().Edges()
	chamferEdges := allEdges.IsCircle().Matching(func(e *sketch.Edge) bool { return e.CircleRadius() == 5.8/2. })

	chamferEdges = append(chamferEdges, allEdges.IsCircle().Matching(func(e *sketch.Edge) bool {
		radius := e.CircleRadius()
		z := e.FirstVertex().Z()
		return (radius == 6 && z == 15) || (radius == 35/2. && z == 25)
	})...)

	block, err = cad.Chamfer(viseBlock.Shape(), chamferEdges, 1.5)
	if err != nil {
		fmt.Println("Failed to chamfer the vise block")
		return
	}

	// Fillet box edges
	block, err = cad.Fillet(block, filletEdges, 3)
	if err != nil {
		fmt.Println("Failed to fillet the vise block")
		return
	}

	exports := makercad.ListOfShape{block}
	// exports := makercad.ListOfShape{viseBlock.Shape()}
	cad.ExportStl("miwa-lix-vise-block.stl", exports, makercad.QualityHigh)
	cad.ExportStep("miwa-lix-vise-block.step", exports)
}
