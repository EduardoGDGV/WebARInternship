import { MediaUpload, MediaUploadCheck } from '@wordpress/block-editor';
import { Button } from '@wordpress/components';
import { __ } from '@wordpress/i18n';

export function MediaUploadButton({ onSelect }) {
    return (
        <MediaUploadCheck>
            <MediaUpload onSelect={onSelect} render={({ open }) => (
                <Button isSecondary onClick={open}>{ __('Choose 3D Asset (Upload or Library)', 'wp-3d-asset-editor') }</Button>
            )} />
        </MediaUploadCheck>
    );
}