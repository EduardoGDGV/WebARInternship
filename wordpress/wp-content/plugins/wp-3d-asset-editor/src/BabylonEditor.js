import { useEffect, useRef, useCallback } from 'react';
import { Engine, Scene, ArcRotateCamera, HemisphericLight, MeshBuilder, Vector3, Quaternion, TransformNode } from '@babylonjs/core';

export default function BabylonEditor({ lat = 0, lng = 0, yaw = 0, pitch = 0, roll = 0, onTransformChange }) {
    const canvasRef = useRef(null);
    const engineRef = useRef(null);
    const sceneRef = useRef(null);
    const meshRef = useRef(null);

    const METERS_PER_DEG = 111320;
    const METERS_TO_SCENE = 1 / 10;
    const ORIGIN = { lat: 0, lng: 0 };

    const latLngToXZ = (lat, lng) => {
        const dx = (lng - ORIGIN.lng) * METERS_PER_DEG * Math.cos((ORIGIN.lat * Math.PI) / 180);
        const dz = (lat - ORIGIN.lat) * METERS_PER_DEG;
        return { x: dx * METERS_TO_SCENE, z: dz * METERS_TO_SCENE };
    };

    // Initialize Babylon scene
    useEffect(() => {
        if (!canvasRef.current || engineRef.current) return;

        const engine = new Engine(canvasRef.current, true, { alpha: true });
        const scene = new Scene(engine);
        engineRef.current = engine;
        sceneRef.current = scene;

        const camera = new ArcRotateCamera('cam', -Math.PI / 2, Math.PI / 2.5, 20, Vector3.Zero(), scene);
        camera.attachControl(canvasRef.current, true);

        new HemisphericLight('light', new Vector3(0, 1, 0), scene);
        MeshBuilder.CreateGround('ground', { width: 200, height: 200 }, scene);

        // Create cube
        const cube = MeshBuilder.CreateBox('cube', { size: 2 }, scene);
        cube.position = new Vector3(...Object.values(latLngToXZ(lat, lng)));
        cube.rotationQuaternion = Quaternion.FromEulerAngles((pitch * Math.PI)/180, (yaw * Math.PI)/180, (roll * Math.PI)/180);
        meshRef.current = cube;

        // Drag/rotate handlers
        let isDragging = false;
        let startPos = null;

        const pointerDown = (evt) => {
            isDragging = true;
            startPos = evt.pickInfo?.pickedPoint.clone();
        };

        const pointerUp = () => {
            isDragging = false;
            startPos = null;
            // Send updated coordinates to block
            if (meshRef.current && onTransformChange) {
                const newLat = ORIGIN.lat + meshRef.current.position.z / METERS_TO_SCENE / METERS_PER_DEG;
                const newLng = ORIGIN.lng + meshRef.current.position.x / METERS_TO_SCENE / (METERS_PER_DEG * Math.cos((ORIGIN.lat * Math.PI)/180));
                const euler = meshRef.current.rotationQuaternion.toEulerAngles();
                onTransformChange({
                    lat: newLat,
                    lng: newLng,
                    yaw: (euler.y * 180)/Math.PI,
                    pitch: (euler.x * 180)/Math.PI,
                    roll: (euler.z * 180)/Math.PI
                });
            }
        };

        const pointerMove = (evt) => {
            if (!isDragging || !startPos) return;
            const newPos = evt.pickInfo?.pickedPoint;
            if (newPos) {
                meshRef.current.position.x += newPos.x - startPos.x;
                meshRef.current.position.z += newPos.z - startPos.z;
                startPos = newPos.clone();
            }
        };

        scene.onPointerObservable.add((pointerInfo) => {
            switch (pointerInfo.type) {
                case 0: pointerDown(pointerInfo); break;
                case 1: pointerUp(); break;
                case 2: pointerMove(pointerInfo); break;
            }
        });

        engine.runRenderLoop(() => scene.render());
        window.addEventListener('resize', () => engine.resize());

        return () => {
            engine.stopRenderLoop();
            scene.dispose();
            engine.dispose();
        };
    }, []);

    return <canvas ref={canvasRef} style={{ width: '100%', height: '100%' }} />;
}
