package main

import (
	"fmt"
	"os"

	"github.com/marcuswu/makercad"
	"github.com/marcuswu/makercad/sketcher"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	blockWidth := 60.
	blockHeight := 25.
	lockDia := 35.
	lockDepth := 10.
	boltHoleDia := 5.8
	boltHoleOffset := 13.
	lockCamHeight := 4.
	lockCamRadius := 6.

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	cad := makercad.NewMakerCad()
	// Create the initial box shape
	block := cad.MakeBox(cad.TopPlane, blockWidth, blockWidth, blockHeight, true)

	// save the block's edges for later fillets
	filletEdges := block.Faces().Edges()

	// Cut recess for lock
	// Find the top face aligned with Z positive
	faces := block.Faces().AlignedWith(cad.TopPlane)
	faces.SortByZ(true)
	topFace := faces[0]
	recessLocation := sketcher.NewPlaneParametersFromCoordinateSystem(topFace.Plane())
	recessLocation.Normal = cad.BottomPlane.Normal
	lockRecess := cad.MakeCylinder(recessLocation, lockDia/2.0, lockDepth)
	viseBlock, err := cad.Remove(block, makercad.ListOfShape{lockRecess})
	if err != nil {
		fmt.Println("Error cutting lock recess")
		return
	}

	// Cut through holes for 5mm bolts
	boltHole := func(block *makercad.CadOperation, location *sketcher.Vector) (*makercad.CadOperation, error) {
		holeLoc := &sketcher.PlaneParameters{
			Location: location,
			Normal:   cad.TopPlane.Normal,
			X:        cad.TopPlane.X,
		}
		holeShape := cad.MakeCylinder(holeLoc, boltHoleDia/2.0, blockHeight)
		return cad.Remove(block.Shape(), makercad.ListOfShape{holeShape})
	}
	viseBlock, err = boltHole(viseBlock, sketcher.NewVectorFromValues(boltHoleOffset, 0, 0))
	if err != nil {
		fmt.Println("Error cutting right bolt hole")
		return
	}
	viseBlock, err = boltHole(viseBlock, sketcher.NewVectorFromValues(-boltHoleOffset, 0, 0))
	if err != nil {
		fmt.Println("Error cutting left bolt hole")
		return
	}

	// Find lock recess surface
	recessSurface := viseBlock.Shape().Faces().AlignedWith(cad.TopPlane).First(func(f *makercad.Face) bool {
		return f.DistanceFrom(0, 0, blockHeight) == lockDepth
	})

	// Cut clearance for lock cam
	camRecessLoc := sketcher.NewPlaneParametersFromCoordinateSystem(recessSurface.Plane())
	camRecessLoc.Location.Z -= lockCamHeight
	camRecess := cad.MakeCylinder(camRecessLoc, lockCamRadius, lockCamHeight)
	viseBlock, err = cad.Remove(viseBlock.Shape(), makercad.ListOfShape{camRecess})
	if err != nil {
		fmt.Println("Error cutting cam recess")
		return
	}

	// Cut channels for wings on the back of the lock
	channel := struct {
		outer float64
		inner float64
		width float64
	}{
		14., // actual
		12., // actual
		17., // guess
	}
	sketch := cad.Sketch(recessSurface)
	l1 := sketch.Line(channel.width, channel.outer, -channel.width, channel.outer)
	arc1 := sketch.Arc(0, 0, -channel.width, channel.outer, -channel.width, channel.inner)
	l2 := sketch.Line(-channel.width, channel.inner, channel.width, channel.inner)
	arc2 := sketch.Arc(0, 0, channel.width, channel.inner, channel.width, channel.outer)

	arc2.End.Coincident(l1.Start)
	l1.End.Coincident(arc1.Start)
	arc1.End.Coincident(l2.Start)
	l2.End.Coincident(arc2.Start)

	arc1.Diameter(lockDia).Center.Coincident(sketch.Origin())
	arc2.Diameter(lockDia).Center.Coincident(sketch.Origin())

	l1.Horizontal()
	l2.Horizontal()

	sketch.Origin().Distance(l1, channel.outer)
	sketch.Origin().Distance(l2, channel.inner)

	err = sketch.Solve()
	if err != nil {
		fmt.Println("Error solving sketch for lock wings")
		return
	}

	sketch.ExportImage("sketcher.svg")

	face1 := makercad.NewFace(sketch)
	face2, err := face1.Mirror(cad.FrontPlane)
	if err != nil {
		fmt.Println("Error mirroring face for lock wings")
		return
	}

	mergeTo := makercad.ListOfShape{viseBlock.Shape()}
	viseBlock = face1.ExtrudeMerging(-2, makercad.MergeTypeRemove, mergeTo.ToCascadeList())

	mergeTo = makercad.ListOfShape{viseBlock.Shape()}
	viseBlock = face2.ExtrudeMerging(2, makercad.MergeTypeRemove, mergeTo.ToCascadeList())

	// find faces to chamfer
	allEdges := viseBlock.Shape().Faces().Edges()
	chamferEdges := allEdges.IsCircle().Matching(func(e *sketcher.Edge) bool { return e.CircleRadius() == 5.8/2. })

	chamferEdges = append(chamferEdges, allEdges.IsCircle().Matching(func(e *sketcher.Edge) bool {
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
