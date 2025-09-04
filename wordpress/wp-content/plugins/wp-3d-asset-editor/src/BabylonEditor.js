import { useEffect, useRef, useState, useCallback } from 'react';
import { Engine, Scene, ArcRotateCamera, HemisphericLight, MeshBuilder, TransformNode, Vector3, Quaternion, SceneLoader } from '@babylonjs/core';
import '@babylonjs/loaders/glTF';

// Simple equirectangular-ish mapping
const ORIGIN = { lat: 0, lng: 0 };
const METERS_PER_DEG = 111_320; // meters per degree latitude
const METERS_TO_SCENE = 1 / 10;  // 10 meters -> 1 scene unit

function latLngToXZ(lat, lng) {
  const dLat = (lat - ORIGIN.lat) * METERS_PER_DEG;
  const dLng = (lng - ORIGIN.lng) * (METERS_PER_DEG * Math.cos((ORIGIN.lat * Math.PI) / 180));
  return { x: dLng * METERS_TO_SCENE, z: dLat * METERS_TO_SCENE };
}

export default function BabylonEditor({ modelUrl = '', lat = 0, lng = 0, yaw = 0, pitch = 0, roll = 0 }) {
  const canvasRef = useRef(null);
  const engineRef = useRef(null);
  const sceneRef = useRef(null);
  const meshRootRef = useRef(null);

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  // Initialize Babylon scene once
  useEffect(() => {
    if (!canvasRef.current || engineRef.current) return;

    const engine = new Engine(canvasRef.current, true, { alpha: true, adaptToDeviceRatio: true });
    engineRef.current = engine;

    const scene = new Scene(engine);
    sceneRef.current = scene;

    const camera = new ArcRotateCamera('camera', -Math.PI / 2, Math.PI / 2.5, 20, Vector3.Zero(), scene);
    camera.attachControl(canvasRef.current, true);

    new HemisphericLight('light', new Vector3(0, 1, 0), scene);
    MeshBuilder.CreateGround('ground', { width: 200, height: 200 }, scene);

    engine.runRenderLoop(() => scene.render());

    const onResize = () => engine.resize();
    window.addEventListener('resize', onResize);

    return () => {
      window.removeEventListener('resize', onResize);
      engine.stopRenderLoop();
      scene.dispose();
      engine.dispose();
      engineRef.current = null;
      sceneRef.current = null;
    };
  }, []);

  // Apply position & rotation
  const applyTransform = useCallback(
    (latV, lngV, yawV, pitchV, rollV) => {
      if (!meshRootRef.current) return;
      const { x, z } = latLngToXZ(Number(latV) || 0, Number(lngV) || 0);
      meshRootRef.current.position = new Vector3(x, 0, z);

      const qYaw = Quaternion.FromEulerAngles(0, (yawV * Math.PI) / 180, 0);
      const qPitch = Quaternion.FromEulerAngles((pitchV * Math.PI) / 180, 0, 0);
      const qRoll = Quaternion.FromEulerAngles(0, 0, (rollV * Math.PI) / 180);

      meshRootRef.current.rotationQuaternion = qYaw.multiply(qPitch).multiply(qRoll);
    },
    []
  );

  // Load 3D model
  const loadModel = useCallback(async () => {
    if (!sceneRef.current) return;
    setLoading(true);
    setError('');

    if (meshRootRef.current) {
      meshRootRef.current.getChildMeshes().forEach(m => m.dispose());
      meshRootRef.current.dispose();
      meshRootRef.current = null;
    }

    try {
      const root = new TransformNode('assetRoot', sceneRef.current);

      if (modelUrl) {
        const result = await SceneLoader.ImportMeshAsync(null, modelUrl, '', sceneRef.current);
        const rootMesh = result.meshes[0];
        if (rootMesh) rootMesh.setParent(root);

        // Auto-scale model to fit scene
        const { min, max } = rootMesh.getHierarchyBoundingVectors();
        const size = max.subtract(min);
        const maxDim = Math.max(size.x, size.y, size.z) || 1;
        const scale = 5 / maxDim;
        root.scaling = new Vector3(scale, scale, scale);
      }

      meshRootRef.current = root;
      applyTransform(lat, lng, yaw, pitch, roll);
    } catch (e) {
      console.error(e);
      setError('Failed to load model (check URL and CORS).');
    } finally {
      setLoading(false);
    }
  }, [modelUrl, applyTransform, lat, lng, yaw, pitch, roll]);

  // Reload model when URL changes
  useEffect(() => {
    loadModel();
  }, [loadModel]);

  // Reapply transform when attributes change
  useEffect(() => {
    applyTransform(lat, lng, yaw, pitch, roll);
  }, [lat, lng, yaw, pitch, roll, applyTransform]);

  return <canvas ref={canvasRef} style={{ width: '100%', height: '100%' }} />;
}
