import { Engine, Scene, ArcRotateCamera, Vector3, HemisphericLight, MeshBuilder, StandardMaterial } from "@babylonjs/core";
import { SceneLoader } from "@babylonjs/core/Loading/sceneLoader";
import "@babylonjs/loaders/glTF";

function getParams() {
  const urlParams = new URLSearchParams(window.location.search);
  return {
    assetUrl: urlParams.get("assetUrl"),
    posX: parseFloat(urlParams.get("posX") || 0),
    posY: parseFloat(urlParams.get("posY") || 0),
    posZ: parseFloat(urlParams.get("posZ") || 0),
    rotX: parseFloat(urlParams.get("rotX") || 0),
    rotY: parseFloat(urlParams.get("rotY") || 0),
    rotZ: parseFloat(urlParams.get("rotZ") || 0),
  };
}

const params = getParams();

const canvas = document.getElementById("renderCanvas");
const engine = new Engine(canvas, true);
const scene = new Scene(engine);

// Camera + light
const camera = new ArcRotateCamera("camera", Math.PI / 2, Math.PI / 4, 15, Vector3.Zero(), scene);
camera.attachControl(canvas, true);
new HemisphericLight("light", new Vector3(0, 1, 0), scene);

// Ground plane
const ground = MeshBuilder.CreateGround("ground", { width: 30, height: 30 }, scene);
const groundMat = new StandardMaterial("groundMat", scene);
groundMat.diffuseColor.set(0.2, 0.2, 0.25);
ground.material = groundMat;

// Load the model (or cube fallback)
(async () => {
  try {
    if (params.assetUrl) {
      const url = new URL(params.assetUrl, window.location.origin);
      const root = url.href.substring(0, url.href.lastIndexOf("/") + 1);
      const file = url.pathname.split("/").pop();

      const result = await SceneLoader.ImportMeshAsync("", root, file, scene);
      const mesh = result.meshes[0];
      mesh.position = new Vector3(params.posX, params.posY, params.posZ);
      mesh.rotation = new Vector3(params.rotX, params.rotY, params.rotZ);
    } else {
      const cube = MeshBuilder.CreateBox("cube", { size: 1 }, scene);
      cube.position = new Vector3(params.posX, params.posY, params.posZ);
      cube.rotation = new Vector3(params.rotX, params.rotY, params.rotZ);
    }
    document.getElementById("loading").style.display = "none";
  } catch (e) {
    console.error(e);
    document.getElementById("loading").textContent = "Failed to load model.";
  }
})();

engine.runRenderLoop(() => scene.render());
window.addEventListener("resize", () => engine.resize());
