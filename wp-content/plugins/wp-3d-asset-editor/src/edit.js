import { useEffect, useRef, useState } from '@wordpress/element';
import { useBlockProps } from '@wordpress/block-editor';
import { createScene } from './scene/initBabylon.js';
import { createOffscreenMap, renderMapToDynamicTexture } from './scene/leafletTexture.js';
import { loadAssetMesh } from './scene/assetLoader.js';
import { MediaUploadButton } from './scene/uiControls.js';
import './css/style.css'; // external styling

// Main Edit component
export default function Edit({ attributes, setAttributes }) {
    const {
        assetUrl,
        blockAssetId,
        posX = 0, posY = 0, posZ = 0,
        rotX = 0, rotY = 0, rotZ = 0
    } = attributes;

    const canvasRef = useRef(null);
    const sceneCtxRef = useRef(null);

    // Initialize Babylon scene + offscreen map once
    useEffect(() => {
        let ctx = null;
        let mapContainer = null;
        let canceled = false;

        const initScene = () => {
            if (!canvasRef.current || !canvasRef.current.offsetParent) {
                // Canvas not yet attached to DOM, retry next frame
                requestAnimationFrame(initScene);
                return;
            }

            try {
                ctx = createScene(canvasRef.current);
                sceneCtxRef.current = ctx;

                const mapObj = createOffscreenMap();
                mapContainer = mapObj.mapContainer;
                if (!canceled && ctx && ctx.mapTexture && mapObj.map) {
                    renderMapToDynamicTexture(mapObj.map, ctx.mapTexture);
                }
            } catch (e) {
                console.error('Failed to initialize Babylon scene:', e);
            }
        };

        initScene();

        return () => {
            canceled = true;
            if (ctx) try { ctx.dispose(); } catch(e) {}
            if (mapContainer && mapContainer.parentNode) try { document.body.removeChild(mapContainer); } catch(e) {}
        };
    }, []);

    // Load selected asset mesh dynamically
    useEffect(() => {
        let aborted = false;
        (async () => {
            const ctx = sceneCtxRef.current;
            if (!ctx || !assetUrl) return;
            try {
                // clear previous mesh if any
                if (ctx.blockMesh) {
                    try { ctx.blockMesh.dispose(); } catch(e) {}
                }

                const mesh = await loadAssetMesh(assetUrl, ctx.scene);
                if (aborted) return;

                mesh.isPickable = true;
                mesh.position.set(posX, posY, posZ);
                mesh.rotation.set(rotX, rotY, rotZ);

                ctx.blockMesh = mesh;
                // TODO: attach gizmo or transform controls here
            } catch (e) {
                console.warn('Mesh load failed:', e);
            }
        })();
        return () => { aborted = true; };
    }, [assetUrl, posX, posY, posZ, rotX, rotY, rotZ]);

    // Handle file upload and set attributes
    const handleAssetUpload = (media) => {
        setAttributes({ assetUrl: media.url });
    };

    // Gutenberg block UI
    return (
        <div {...useBlockProps({ className: 'wp-3d-asset-editor' })}>
            <MediaUploadButton onSelect={handleAssetUpload} />
            <div className="editor-header">
                <strong>Block asset preview & controls</strong>
                <p>Move/rotate, then Save post to persist. Saved blocks render read-only together for context.</p>
            </div>
            <div className="editor-preview">
                <canvas ref={canvasRef} width={900} height={600} className="asset-canvas" />
            </div>
        </div>
    );
}
