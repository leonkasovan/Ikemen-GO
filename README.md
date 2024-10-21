# Ikemen GO

Ikemen GO is an open source fighting game engine that supports resources from the [M.U.G.E.N](https://en.wikipedia.org/wiki/Mugen_(game_engine)) engine, written in Google’s programming language, [Go](https://go.dev/). It is a complete rewrite of a prior engine known simply as Ikemen.

## Features
Ikemen GO aims for backwards-compatibility on par with M.U.G.E.N version 1.1 Beta, while simultaneously expanding on its features in a variety of ways.

Refer to [our wiki](https://github.com/ikemen-engine/Ikemen-GO/wiki) to see a comprehensive list of new features that have been added in Ikemen GO.

## Installing
Ready to use builds for Windows, macOS and Linux can be found in the [releases section](https://github.com/ikemen-engine/Ikemen-GO/releases) of this repository. You can find nightly builds [here](https://github.com/ikemen-engine/Ikemen-GO/releases/tag/nightly) as well, which update on every commit.

## Running
Download the ZIP archive that matches your operating system and extract its contents to your preferred location.

On Windows, double-click `Ikemen_GO.exe` (`Ikemen_GO_x86.exe` on 32-bit OSes).
On macOS or Linux, double-click `Ikemen_GO.command`.

## Developing
These instructions are for those interested in developing the Ikemen GO engine itself. Instructions for creating custom stages, fonts, characters and other resources can be found in the community forum.

### Building
You can find instructions for building Ikemen GO on our wiki. Instructions are available for [Windows](https://github.com/ikemen-engine/Ikemen-GO/wiki/Building,-Installing-and-Distributing#building-on-windows), [macOS](https://github.com/ikemen-engine/Ikemen-GO/wiki/Building,-Installing-and-Distributing#building-on-macos), and [Linux](https://github.com/ikemen-engine/Ikemen-GO/wiki/Building,-Installing-and-Distributing#building-on-linux).

Summarize Ikemen Batch Engine 
```
// Collecting RenderParams for RenderSprite
Sprite.draw(image.go) -> CalculateRenderData(render.go) -> BatchParam(render.go): append sys.paramList
Fnt.drawChar(font.go) -> CalculateRenderData(render.go) -> BatchParam(render.go): append sys.paramList
ClsnRect.drawChar(char.go) -> CalculateRenderData(render.go) -> BatchParam(render.go): append sys.paramList
Animation.Draw(anim.go) -> CalculateRenderData(render.go) -> BatchParam(render.go): append sys.paramList
Animation.ShadowDraw(anim.go) -> CalculateRenderData(render.go) -> BatchParam(render.go): append sys.paramList

// Collecting RenderParams for FillRect
systemScriptInit(script.go) -> luaRegister: "clearColor", "fade", "fadeColor", "fillRect" -> CalculateRectData(render.go) -> BatchParam(render.go): append sys.paramList
System.draw(system.go) -> CalculateRectData(render.go) -> BatchParam(render.go): append sys.paramList
System.drawTop(system.go) -> CalculateRectData(render.go) -> BatchParam(render.go): append sys.paramList

// Batch Processing RenderParams 
systemScriptInit(script.go) -> luaRegister("game") -> System.await(system.go) -> BatchRender(render.go) -> processBatch : iterate sys.paramList

// Stage draw
systemScriptInit(script.go) -> luaRegister("game") -> System.fight(system.go) -> Stage.draw(stage.go) -> Stage.drawModel(stage.go) -> drawNode(stage.go)
```

## License
Ikemen GO's source code is available under the MIT License. Certain non-code assets are licensed under CC-BY 3.0.

See [License.txt](License.txt) for more details.
