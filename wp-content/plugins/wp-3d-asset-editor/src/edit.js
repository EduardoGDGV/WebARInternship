// edit.js
import { useEffect, useRef, useState } from '@wordpress/element';
import { __ } from '@wordpress/i18n';
import { useBlockProps, MediaUpload, MediaUploadCheck } from '@wordpress/block-editor';
import { Button } from '@wordpress/components';
import { Engine, Scene, ArcRotateCamera, HemisphericLight, Vector3, MeshBuilder, StandardMaterial, DynamicTexture } from "@babylonjs/core";
import { GizmoManager } from "@babylonjs/core/Gizmos/gizmoManager.js";
import { SceneLoader } from "@babylonjs/core/Loading/sceneLoader";
import "@babylonjs/loaders/glTF";
import L from 'leaflet';
import leafletImage from 'leaflet-image';
import 'leaflet/dist/leaflet.css';

export default function Edit({ attributes, setAttributes }) {
    const {
        blockAssetId,
        assetUrl = '',
        posX = 0, posY = 0, posZ = 0,
        rotX = 0, rotY = 0, rotZ = 0
    } = attributes;

    const blockProps = useBlockProps();
    const canvasRef = useRef(null);

    // Engine/scene refs
    const engineRef = useRef(null);
    const sceneRef = useRef(null);

    // Mesh refs
    const savedMeshesRef = useRef({}); // map assetId -> mesh
    const blockMeshRef = useRef(null);
    const gizmoManagerRef = useRef(null);

    // Data
    const [assets, setAssets] = useState([]); // list of saved 3d_asset posts

    // Prevent duplicate saves and detect saving transitions
    const savingInProgressRef = useRef(false);
    const prevIsSavingRef = useRef(false);

    // --- fetch all saved 3d_asset posts once, and refresh when changed
    useEffect(() => {
        let mounted = true;
        const loadAssets = async () => {
            try {
                const resp = await wp.apiFetch({ path: '/wp/v2/3d_asset?per_page=100' });
                if (!mounted) return;
                setAssets(Array.isArray(resp) ? resp : []);
            } catch (e) {
                console.error('Failed to fetch 3d_asset list:', e);
                setAssets([]);
            }
        };
        loadAssets();

        // re-fetch whenever a post is saved/published (so the scene updates)
        const unsubscribe = wp.data.subscribe(() => {
            const isSaving = wp.data.select('core/editor').isSavingPost();
            const isAutosaving = wp.data.select('core/editor').isAutosavingPost();
            const isPublishing = wp.data.select('core/editor').isPublishingPost();
            if ((isSaving || isPublishing) && !isAutosaving) {
                loadAssets();
            }
        });

        return () => {
            mounted = false;
            unsubscribe();
        };
    }, []);

    // --- Initialize Babylon engine & scene only once
    useEffect(() => {
        if (!canvasRef.current) return;

        const engine = new Engine(canvasRef.current, true);
        const scene = new Scene(engine);
        engineRef.current = engine;
        sceneRef.current = scene;

        // camera / light
        const camera = new ArcRotateCamera("camera", Math.PI / 2, Math.PI / 4, 10, Vector3.Zero(), scene);
        camera.attachControl(canvasRef.current, true);
        new HemisphericLight("light", new Vector3(0, 1, 0), scene);

        // gizmo manager
        const gizmoManager = new GizmoManager(scene);
        gizmoManager.positionGizmoEnabled = true;
        gizmoManager.rotationGizmoEnabled = true;
        gizmoManagerRef.current = gizmoManager;

        // Leaflet ground plane
        const groundSize = 50; // meters
        const ground = MeshBuilder.CreatePlane("ground", { size: groundSize }, scene);
        ground.rotation.x = Math.PI / 2; // horizontal
        ground.position.y = 0;
        ground.isPickable = false;
        ground.freezeWorldMatrix();

        // Dynamic texture for Leaflet map
        const groundMat = new StandardMaterial("groundMat", scene);
        const mapTexture = new DynamicTexture("mapTex", { width: 1024, height: 1024 }, scene, true);
        groundMat.diffuseTexture = mapTexture;
        ground.material = groundMat;

        // Create offscreen Leaflet map container
        const mapContainer = document.createElement("div");
        mapContainer.id = "leaflet-map-offscreen";
        mapContainer.style.width = "1024px";
        mapContainer.style.height = "1024px";
        mapContainer.style.position = "absolute";
        mapContainer.style.top = "-9999px";
        document.body.appendChild(mapContainer);

        // Initialize Leaflet map at your coords
        const map = L.map(mapContainer, {
            center: [-23.55719, -46.73015],
            zoom: 32,
            maxBounds: [[-200, -300], [200, 300]],
            minZoom: 1,
            worldCopyJump: true,
            zoomControl: false,
            dragging: false,
            scrollWheelZoom: false,
            doubleClickZoom: false,
            boxZoom: false,
            keyboard: false,
            touchZoom: false,
            attributionControl: false,
            interactive: false
        });

        // Add OpenStreetMap tiles
        L.tileLayer("https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png", {
            attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
            subdomains: ["a", "b", "c"],
            crossOrigin: true
        }).addTo(map);

        // Render Leaflet into Babylon texture
        const renderMapTexture = () => {
            leafletImage(map, (err, leafletCanvas) => {
                if (!err && leafletCanvas) {
                const ctx = mapTexture.getContext();
                if (ctx) {
                    ctx.drawImage(leafletCanvas, 0, 0, 1024, 1024);
                    mapTexture.update();
                } else {
                    console.error("DynamicTexture context not available.");
                }
                } else {
                console.error("Leaflet image error:", err);
                }
            });
        };

        renderMapTexture();

        engine.runRenderLoop(() => scene.render());
        const onResize = () => engine.resize();
        window.addEventListener('resize', onResize);

        return () => {
            window.removeEventListener('resize', onResize);
            try { gizmoManager.dispose(); } catch (e) {}
            try { engine.dispose(); } catch (e) {}
            document.body.removeChild(mapContainer);
        };
    }, []);

    // --- Render saved assets into scene (read-only)
    useEffect(() => {
        const scene = sceneRef.current;
        if (!scene) return;

        const ensureMeshForAsset = async (asset) => {
            const id = asset.id;
            const existing = savedMeshesRef.current[id];
            if (existing && !existing.isDisposed()) return existing;

            const meta = asset.meta || {};
            let mesh = null;
            try {
                if (meta.assetUrl) {
                    const url = new URL(meta.assetUrl, window.location.origin);
                    const root = url.href.substring(0, url.href.lastIndexOf("/") + 1);
                    const file = url.pathname.split("/").pop();
                    const result = await SceneLoader.ImportMeshAsync("", root, file, scene);
                    mesh = result.meshes.find(m => m && typeof m.getTotalVertices === 'function') || result.meshes[0];
                    if (mesh && mesh.rotationQuaternion) {
                        const e = mesh.rotationQuaternion.toEulerAngles();
                        mesh.rotation = new Vector3(e.x, e.y, e.z);
                        mesh.rotationQuaternion = null;
                    }
                }
            } catch (e) {
                console.warn(`Failed to load saved asset #${id}, using fallback cube`, e);
            }
            if (!mesh || mesh.isDisposed()) {
                mesh = MeshBuilder.CreateBox(`saved-asset-${id}`, { size: 1 }, scene);
            }
            mesh.isPickable = false;
            mesh.position = new Vector3(parseFloat(meta.posX || 0), parseFloat(meta.posY || 0), parseFloat(meta.posZ || 0));
            mesh.rotation = new Vector3(parseFloat(meta.rotX || 0), parseFloat(meta.rotY || 0), parseFloat(meta.rotZ || 0));
            mesh.freezeWorldMatrix();
            savedMeshesRef.current[id] = mesh;
            return mesh;
        };

        (async () => {
            const existingIds = new Set(assets.map(a => a.id));
            for (const idStr of Object.keys(savedMeshesRef.current)) {
                const id = parseInt(idStr, 10);
                if (!existingIds.has(id)) {
                    const m = savedMeshesRef.current[id];
                    try { m.dispose(); } catch (e) {}
                    delete savedMeshesRef.current[id];
                }
            }

            for (const asset of assets) {
                if (asset.id === blockAssetId) continue;
                await ensureMeshForAsset(asset);
            }
        })();
    }, [assets]);

    // --- Create / update the block's own mesh (movable)
    useEffect(() => {
        const scene = sceneRef.current;
        const gizmoManager = gizmoManagerRef.current;
        if (!scene || !gizmoManager) return;

        try {
            if (blockMeshRef.current && !blockMeshRef.current.isDisposed()) {
                gizmoManager.attachToMesh(null);
                blockMeshRef.current.dispose();
            }
        } catch (e) {}

        (async () => {
            let mesh = null;
            if (assetUrl) {
                try {
                    const url = new URL(assetUrl, window.location.origin);
                    const root = url.href.substring(0, url.href.lastIndexOf("/") + 1);
                    const file = url.pathname.split("/").pop();
                    const result = await SceneLoader.ImportMeshAsync("", root, file, scene);
                    mesh = result.meshes.find(m => m && typeof m.getTotalVertices === 'function') || result.meshes[0];
                    if (mesh && mesh.rotationQuaternion) {
                        const e = mesh.rotationQuaternion.toEulerAngles();
                        mesh.rotation = new Vector3(e.x, e.y, e.z);
                        mesh.rotationQuaternion = null;
                    }
                } catch (e) {
                    console.warn('Failed to load block asset, using cube', e);
                }
            }
            if (!mesh) {
                mesh = MeshBuilder.CreateBox('block-mesh', { size: 1 }, scene);
            }
            mesh.isPickable = true;
            mesh.position = new Vector3(posX || 0, posY || 0, posZ || 0);
            mesh.rotation = new Vector3(rotX || 0, rotY || 0, rotZ || 0);
            blockMeshRef.current = mesh;

            gizmoManager.attachToMesh(mesh);

            const updateBlockAttributes = () => {
                const m = blockMeshRef.current;
                if (!m) return;
                setAttributes({
                    assetUrl: assetUrl || '',
                    blockAssetId: blockAssetId || null,
                    posX: m.position.x,
                    posY: m.position.y,
                    posZ: m.position.z,
                    rotX: m.rotation.x,
                    rotY: m.rotation.y,
                    rotZ: m.rotation.z,
                });
            };

            gizmoManager.gizmos.positionGizmo?.onDragEndObservable.add(updateBlockAttributes);
            gizmoManager.gizmos.rotationGizmo?.onDragEndObservable.add(updateBlockAttributes);
        })();
    }, [assetUrl, posX, posY, posZ, rotX, rotY, rotZ, blockAssetId]);

    // --- Handle file insert (upload/select) via MediaUpload
    const handleAssetUpload = (media) => {
        setAttributes({
            assetUrl: media.url,
        });
    };

    // --- Save or update the 3d_asset for this block when the post is saved/published
    useEffect(() => {
        const unsub = wp.data.subscribe(async () => {
            try {
                const isSaving = wp.data.select('core/editor').isSavingPost();
                const isAutosaving = wp.data.select('core/editor').isAutosavingPost();
                const isPublishing = wp.data.select('core/editor').isPublishingPost();

                if ((isSaving || isPublishing) && !isAutosaving && !prevIsSavingRef.current) {
                    const mesh = blockMeshRef.current;
                    if (!mesh) {
                        prevIsSavingRef.current = isSaving;
                        return;
                    }

                    if (savingInProgressRef.current) {
                        prevIsSavingRef.current = isSaving;
                        return;
                    }
                    savingInProgressRef.current = true;

                    const rot = mesh.rotationQuaternion ? mesh.rotationQuaternion.toEulerAngles() : mesh.rotation;
                    const payload = {
                        title: `Block Asset #${wp.data.select('core/editor').getCurrentPostId()}`,
                        status: 'publish',
                        meta: {
                            assetUrl: assetUrl || '',
                            posX: +mesh.position.x || 0,
                            posY: +mesh.position.y || 0,
                            posZ: +mesh.position.z || 0,
                            rotX: +rot.x || 0,
                            rotY: +rot.y || 0,
                            rotZ: +rot.z || 0,
                        }
                    };

                    try {
                        let returned;
                        if (!blockAssetId) {
                            returned = await wp.apiFetch({
                                path: '/wp/v2/3d_asset',
                                method: 'POST',
                                data: payload
                            });
                            if (returned && returned.id) {
                                setAttributes({ blockAssetId: returned.id });
                                setAssets(prev => [...prev, returned]);
                            }
                        } else {
                            returned = await wp.apiFetch({
                                path: `/wp/v2/3d_asset/${blockAssetId}`,
                                method: 'POST',
                                data: payload
                            });
                            if (returned && returned.id) {
                                setAssets(prev => prev.map(a => a.id === returned.id ? returned : a));
                            }
                        }
                    } catch (err) {
                        console.error('[wp3d] Failed to create/update 3d_asset:', err);
                    } finally {
                        savingInProgressRef.current = false;
                    }
                }
                prevIsSavingRef.current = wp.data.select('core/editor').isSavingPost();
            } catch (err) {
                console.error('[wp3d] subscribe callback error:', err);
            }
        });
        return () => unsub();
    }, [assetUrl, blockAssetId]);

    // --- UI
    return (
        <div {...blockProps}>
            <MediaUploadCheck>
                <MediaUpload
                    onSelect={handleAssetUpload}
                    render={({ open }) => (
                        <Button isSecondary onClick={open}>
                            { __('Choose 3D Asset (Upload or Select from Library)', 'wp-3d-asset-editor') }
                        </Button>
                    )}
                />
            </MediaUploadCheck>

            <div style={{ marginTop: 12 }}>
                <strong>{ __('Block asset preview & controls (move/rotate then Save post to persist)', 'wp-3d-asset-editor') }</strong>
                <p style={{ marginTop: 6, color: '#666' }}>
                    { __('All saved 3D assets render read-only in the same scene for context.', 'wp-3d-asset-editor') }
                </p>
            </div>

            <canvas
                ref={canvasRef}
                width="900"
                height="600"
                style={{ border: "1px solid #ddd", display: 'block', marginTop: 12 }}
            />
        </div>
    );
}
