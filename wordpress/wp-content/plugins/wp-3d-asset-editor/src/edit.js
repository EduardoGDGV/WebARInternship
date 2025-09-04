import { useState, Suspense } from 'react';
import { __ } from '@wordpress/i18n';
import { useBlockProps, InspectorControls } from '@wordpress/block-editor';
import {
  PanelBody,
  TextControl,
  __experimentalNumberControl as NumberControl
} from '@wordpress/components';

// Lazy load BabylonEditor
const BabylonEditor = React.lazy(() => import('./BabylonEditor'));

export default function Edit({ attributes, setAttributes }) {
  const { modelUrl, lat, lng, yaw, pitch, roll } = attributes;
  const [showEditor] = useState(true);

  const blockProps = useBlockProps({
    style: { width: '100%', height: '400px' },
  });

  return (
    <>
      <InspectorControls>
        <PanelBody title={__('Asset Settings', 'wp3d-asset-editor')}>
          <TextControl
            label={__('Model URL', 'wp3d-asset-editor')}
            value={modelUrl}
            onChange={(v) => setAttributes({ modelUrl: v })}
          />
          <NumberControl
            label={__('Latitude', 'wp3d-asset-editor')}
            value={lat}
            onChange={(v) => setAttributes({ lat: parseFloat(v) || 0 })}
          />
          <NumberControl
            label={__('Longitude', 'wp3d-asset-editor')}
            value={lng}
            onChange={(v) => setAttributes({ lng: parseFloat(v) || 0 })}
          />
          <NumberControl
            label={__('Yaw (°)', 'wp3d-asset-editor')}
            value={yaw}
            onChange={(v) => setAttributes({ yaw: parseFloat(v) || 0 })}
          />
          <NumberControl
            label={__('Pitch (°)', 'wp3d-asset-editor')}
            value={pitch}
            onChange={(v) => setAttributes({ pitch: parseFloat(v) || 0 })}
          />
          <NumberControl
            label={__('Roll (°)', 'wp3d-asset-editor')}
            value={roll}
            onChange={(v) => setAttributes({ roll: parseFloat(v) || 0 })}
          />
        </PanelBody>
      </InspectorControls>

      <div {...blockProps}>
        <Suspense fallback={<div>{__('Loading 3D Editor…', 'wp3d-asset-editor')}</div>}>
          {showEditor && <BabylonEditor {...attributes} />}
        </Suspense>
      </div>
    </>
  );
}
