import { useEffect, useRef, useState } from '@wordpress/element';
import { __ } from '@wordpress/i18n';
import { useBlockProps, MediaUpload, MediaUploadCheck } from '@wordpress/block-editor';
import { SelectControl, Button } from '@wordpress/components';
import { Engine, Scene, ArcRotateCamera, HemisphericLight, Vector3, MeshBuilder, Quaternion } from "@babylonjs/core";
import { GizmoManager } from "@babylonjs/core/Gizmos/gizmoManager.js";
import { SceneLoader } from "@babylonjs/core/Loading/sceneLoader";
import "@babylonjs/loaders/glTF";

export default function Edit({ attributes, setAttributes }) {
    const { sceneId = 0 } = attributes;
    const blockProps = useBlockProps();
    const canvasRef = useRef(null);
    const [scenes, setScenes] = useState([]);
    const [assets, setAssets] = useState([]);
    const savePending = useRef({});
    const createdDefaultRef = useRef(false);

    // Fetch scenes
    useEffect(() => {
        wp.apiFetch({ path: '/wp/v2/3d_scene?per_page=50' })
            .then(async (data) => {
                if (!Array.isArray(data)) return;

                if (data.length === 0 && !createdDefaultRef.current) {
                    createdDefaultRef.current = true;
                    try {
                        const newScene = await wp.apiFetch({
                            path: '/wp/v2/3d_scene',
                            method: 'POST',
                            data: { title: 'Default Shared Scene', status: 'publish' },
                        });
                        setScenes([newScene]);
                        setAttributes({ sceneId: newScene.id });
                        return;
                    } catch (err) {
                        console.error('Failed to create default scene:', err);
                    }
                }

                setScenes(data);
                if (!sceneId && data[0]) setAttributes({ sceneId: data[0].id });
            })
            .catch(err => console.error('Failed to fetch scenes:', err));
    }, []);

    // Fetch assets for selected scene
    useEffect(() => {
        if (!sceneId) return;
        wp.apiFetch({ path: `/wp/v2/3d_asset?scene_id=${sceneId}&per_page=50` })
            .then(data => setAssets(data || []))
            .catch(err => console.error('Failed to fetch assets:', err));
    }, [sceneId]);

    // Babylon scene setup
    useEffect(() => {
        if (!canvasRef.current || !sceneId) return;

        const engine = new Engine(canvasRef.current, true);
        const scene = new Scene(engine);

        // Camera and light
        const camera = new ArcRotateCamera("camera", Math.PI / 2, Math.PI / 4, 6, Vector3.Zero(), scene);
        camera.attachControl(canvasRef.current, true);
        new HemisphericLight("light", new Vector3(0,1,0), scene);

        const gizmos = [];

        // Save transforms helper
        const saveAssetTransforms = async (assetId, mesh) => {
            if (savePending.current[assetId]) return;
            savePending.current[assetId] = true;
            const rot = mesh.rotationQuaternion ? mesh.rotationQuaternion.toEulerAngles() : mesh.rotation;
            const meta = {
                posX: mesh.position.x,
                posY: mesh.position.y,
                posZ: mesh.position.z,
                rotX: rot.x,
                rotY: rot.y,
                rotZ: rot.z,
            };
            try {
                await wp.apiFetch({
                    path: `/wp/v2/3d_asset/${assetId}`,
                    method: 'POST',
                    data: { meta }
                });
            } catch (e) {
                console.error('Failed to save asset transforms:', e);
            } finally {
                savePending.current[assetId] = false;
            }
        };

        // Load assets
        assets.forEach(asset => {
            (async () => {
                try {
                    if (!asset.meta?.assetUrl) return;

                    const result = await SceneLoader.ImportMeshAsync("", "", asset.meta.assetUrl, scene);
                    const mesh = result.meshes.find(m => m.getTotalVertices) || result.meshes[0];

                    // Apply position & rotation
                    const m = asset.meta;
                    if (mesh.rotationQuaternion) {
                        mesh.rotationQuaternion = Quaternion.FromEulerAngles(
                            m.rotX || 0,
                            m.rotY || 0,
                            m.rotZ || 0
                        );
                    } else {
                        mesh.rotation = new Vector3(m.rotX || 0, m.rotY || 0, m.rotZ || 0);
                    }
                    mesh.position = new Vector3(m.posX || 0, m.posY || 0, m.posZ || 0);

                    // Gizmo
                    const gizmoManager = new GizmoManager(scene);
                    gizmoManager.positionGizmoEnabled = true;
                    gizmoManager.rotationGizmoEnabled = true;
                    gizmoManager.attachToMesh(mesh);
                    gizmoManager.gizmos.positionGizmo?.onDragEndObservable.add(() => saveAssetTransforms(asset.id, mesh));
                    gizmoManager.gizmos.rotationGizmo?.onDragEndObservable.add(() => saveAssetTransforms(asset.id, mesh));
                    gizmos.push(gizmoManager);

                } catch (err) {
                    console.error('Error loading asset:', err);
                }
            })();
        });

        engine.runRenderLoop(() => scene.render());
        const resize = () => engine.resize();
        window.addEventListener("resize", resize);

        return () => {
            window.removeEventListener("resize", resize);
            gizmos.forEach(g => g.dispose());
            engine.dispose();
        };
    }, [assets, sceneId]);

    // Scene select options
    const options = [{ label: __('Select scene'), value: 0 }, ...scenes.map(s => ({
        label: s.title.rendered || `#${s.id}`, value: s.id
    }))];

    // Upload new asset
    const handleAssetUpload = (media) => {
        if (!sceneId) return;
        wp.apiFetch({
            path: '/wp/v2/3d_asset',
            method: 'POST',
            data: {
                title: `Asset for scene #${sceneId}`,
                status: 'publish',
                meta: { assetUrl: media.url, scene_id: sceneId }
            }
        }).then(asset => {
            // Fetch full meta after creation
            wp.apiFetch({ path: `/wp/v2/3d_asset/${asset.id}` })
                .then(full => setAssets(prev => [...prev, full]))
                .catch(err => console.error(err));
        }).catch(err => console.error(err));
    };

    return (
        <div {...blockProps}>
            <SelectControl
                label={__('Select Shared Scene', 'wp-3d-asset-editor')}
                value={sceneId}
                options={options}
                onChange={val => setAttributes({ sceneId: parseInt(val) })}
            />

            <MediaUploadCheck>
                <MediaUpload
                    onSelect={handleAssetUpload}
                    allowedTypes={['model/gltf-binary','model/gltf+json']}
                    render={({ open }) => (
                        <Button onClick={open}>
                            { __('Upload Asset') }
                        </Button>
                    )}
                />
            </MediaUploadCheck>

            <canvas ref={canvasRef} width="800" height="600" style={{ border: "1px solid black", marginTop: "10px" }} />
            <p>{ __('Drag arrows to move, circles to rotate', 'wp-3d-asset-editor') }</p>
        </div>
    );
}
