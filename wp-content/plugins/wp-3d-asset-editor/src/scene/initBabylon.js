import { Engine, Scene, ArcRotateCamera, HemisphericLight, Vector3, MeshBuilder, StandardMaterial, DynamicTexture } from '@babylonjs/core';
import { GizmoManager } from '@babylonjs/core/Gizmos/gizmoManager.js';

console.log('initBabylon.js loaded');

export function createScene(canvas) {
    console.log('Creating Babylon scene on canvas:', canvas);
    // Ensure canvas exists
    if (!canvas) {
        console.warn('No canvas provided to createScene.');
        return null;
    }

    // Check WebGL support
    const gl = canvas.getContext('webgl2') || canvas.getContext('webgl');
    if (!gl) {
        throw new Error('WebGL not supported on this canvas!');
    }

    try {
        const engine = new Engine(canvas, true, {
            preserveDrawingBuffer: true,
            stencil: true,
        });
        const scene = new Scene(engine);
        const camera = new ArcRotateCamera('camera', Math.PI / 2, Math.PI / 4, 10, Vector3.Zero(), scene);
        camera.attachControl(canvas, true);
        new HemisphericLight('hlight', new Vector3(0,1,0), scene);

        // ground plane + dynamic texture placeholder
        const ground = MeshBuilder.CreatePlane('ground', { size: 50 }, scene);
        ground.rotation.x = Math.PI / 2;
        ground.position.y = 0;
        const groundMat = new StandardMaterial('groundMat', scene);
        const mapTexture = new DynamicTexture('mapTex', { width: 1024, height: 1024 }, scene, true);
        groundMat.diffuseTexture = mapTexture;
        ground.material = groundMat;

        const gizmo = new GizmoManager(scene);
        gizmo.positionGizmoEnabled = true;
        gizmo.rotationGizmoEnabled = true;

        engine.runRenderLoop(() => scene.render());
        const onResize = () => engine.resize();
        window.addEventListener('resize', onResize);

        return { engine, scene, camera, ground, mapTexture, gizmo, dispose: () => {
            window.removeEventListener('resize', onResize);
            try { gizmo.dispose(); } catch (e) {}
            try { engine.dispose(); } catch (e) {}
        }};
    } catch (err) {
        console.error('Failed to create Babylon scene:', err);
        // Return a safe fallback object so the rest of your block code doesn't break
        return {
            engine: null,
            scene: null,
            camera: null,
            ground: null,
            mapTexture: null,
            gizmo: null,
            dispose: () => {}
        };
    }
}