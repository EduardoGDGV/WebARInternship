import { useBlockProps } from '@wordpress/block-editor';

export default function save() {
    const blockProps = useBlockProps.save();
    return (
        <div {...blockProps}>
            <canvas id="wp3d-frontend-canvas" width="600" height="400" style={{ border: "1px solid black" }} />
        </div>
    );
}
