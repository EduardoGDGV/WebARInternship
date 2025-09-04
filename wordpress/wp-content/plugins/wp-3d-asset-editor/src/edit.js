import { __ } from '@wordpress/i18n';
import { useBlockProps } from '@wordpress/block-editor';
import { useEffect, useRef } from '@wordpress/element';

// BabylonJS imports
import { Engine, Scene, ArcRotateCamera, HemisphericLight, MeshBuilder, Vector3 } from "@babylonjs/core";
import { GizmoManager } from "@babylonjs/core/Gizmos/gizmoManager.js";

export default function Edit() {
    const blockProps = useBlockProps();
    const canvasRef = useRef(null);

    useEffect(() => {
        if (!canvasRef.current) return;

        console.log("🛠️ Mounting Babylon editor scene…");

        const engine = new Engine(canvasRef.current, true);
        const scene = new Scene(engine);

        // Camera
        const camera = new ArcRotateCamera(
            "camera",
            Math.PI / 2,
            Math.PI / 4,
            6,
            Vector3.Zero(),
            scene
        );
        camera.attachControl(canvasRef.current, true);

        // Light
        new HemisphericLight("light", new Vector3(0, 1, 0), scene);

        // Mesh: simple cube
        const box = MeshBuilder.CreateBox("box", { size: 1 }, scene);

        // 🎮 Gizmo Manager for interaction
        const gizmoManager = new GizmoManager(scene);
        gizmoManager.positionGizmoEnabled = true;  // Move gizmo
        gizmoManager.rotationGizmoEnabled = true;  // Rotate gizmo
        gizmoManager.scaleGizmoEnabled = false;   // Disable scale for now
        gizmoManager.attachToMesh(box);

        // Render loop
        engine.runRenderLoop(() => {
            scene.render();
        });

        // Handle resize
        const onResize = () => engine.resize();
        window.addEventListener("resize", onResize);

        return () => {
            console.log("🧹 Disposing Babylon scene…");
            gizmoManager.dispose();
            window.removeEventListener("resize", onResize);
            engine.dispose();
        };
    }, []);

    return (
        <div {...blockProps}>
            <canvas
                ref={canvasRef}
                width="600"
                height="400"
                style={{ border: "1px solid black" }}
            />
            <p>{ __('Drag arrows to move, circles to rotate', 'wp-3d-asset-editor') }</p>
        </div>
    );
}
