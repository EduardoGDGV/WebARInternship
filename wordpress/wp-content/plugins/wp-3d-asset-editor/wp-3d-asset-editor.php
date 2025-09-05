<?php
/**
 * Plugin Name: WP 3D Asset Editor
 * Description: Basic 3D Scene block with Babylon.js canvas rendering.
 * Version: 1.1.1
 * Author: EduardoGDGV
 */

defined( 'ABSPATH' ) || exit;

/** Register block */
function wp3d_asset_editor_register_block() {
    register_block_type( __DIR__ . '/build' );
}
add_action( 'init', 'wp3d_asset_editor_register_block' );

/** Register CPTs and meta */
function wp3d_register_cpts_and_meta() {
    // Scene CPT
    register_post_type('3d_scene', [
        'label'        => '3D Scenes',
        'public'       => false,
        'show_ui'      => true,
        'show_in_rest' => true,
        'supports'     => ['title'],
    ]);

    // Asset CPT
    register_post_type('3d_asset', [
        'label'        => '3D Assets',
        'public'       => false,
        'show_ui'      => true,
        'show_in_rest' => true,
        'supports'     => ['title'],
    ]);

    // Meta fields for each asset
    register_post_meta('3d_asset', 'assetUrl', [
        'type' => 'string', 'single' => true, 'show_in_rest' => true, 'auth_callback' => '__return_true'
    ]);

    $numeric_fields = ['posX','posY','posZ','rotX','rotY','rotZ'];
    foreach ( $numeric_fields as $field ) {
        register_post_meta('3d_asset', $field, [
            'type' => 'number',
            'single' => true,
            'show_in_rest' => true,
            'auth_callback' => '__return_true'
        ]);
    }

    // Link to scene and original post
    register_post_meta('3d_asset', 'scene_id', [
        'type' => 'integer', 'single' => true, 'show_in_rest' => true, 'auth_callback' => '__return_true'
    ]);
    register_post_meta('3d_asset', 'origin_post', [
        'type' => 'integer', 'single' => true, 'show_in_rest' => true, 'auth_callback' => '__return_true'
    ]);
}
add_action('init', 'wp3d_register_cpts_and_meta');

/** Allow .glb/.gltf uploads */
function wp3d_allow_3d_uploads( $mime_types ) {
    $mime_types['glb']  = 'model/gltf-binary';
    $mime_types['gltf'] = 'model/gltf+json';
    return $mime_types;
}
add_filter( 'upload_mimes', 'wp3d_allow_3d_uploads' );

/** Allow admins unfiltered upload (optional) */
function wp3d_allow_unfiltered_uploads() {
    if ( current_user_can('administrator') ) {
        add_filter('user_has_cap', function($allcaps) {
            $allcaps['unfiltered_upload'] = true;
            return $allcaps;
        }, 0);
    }
}
add_action('init', 'wp3d_allow_unfiltered_uploads');

/** Create a single default shared scene at plugin activation (prevents repeated creation) */
function wp3d_create_default_scene_on_activation() {
    // Check if any scenes exist
    $existing = get_posts([
        'post_type'      => '3d_scene',
        'posts_per_page' => 1,
        'post_status'    => 'any',
        'fields'         => 'ids',
    ]);

    if ( empty($existing) ) {
        $scene_id = wp_insert_post([
            'post_type'   => '3d_scene',
            'post_title'  => 'Default Shared Scene',
            'post_status' => 'publish',
        ]);

        if ( $scene_id && ! is_wp_error($scene_id) ) {
            update_option('wp3d_default_scene_id', (int) $scene_id);
        }
    }
}
register_activation_hook( __FILE__, 'wp3d_create_default_scene_on_activation' );

/**
 * Create a 3d_asset when a normal post with the block is saved.
 * - This function looks for our block in the post content and if found,
 *   will create a 3d_asset post copying the current scene meta (assetUrl/pos/rot).
 * - It sets 'origin_post' meta to avoid duplicating for the same post.
 */
function wp3d_create_asset_from_post( $post_id, $post, $update ) {
    // avoid autosaves, revisions, or non-post types
    if ( wp_is_post_autosave( $post_id ) || wp_is_post_revision( $post_id ) ) return;
    if ( $post->post_type !== 'post' ) return;

    $blocks = parse_blocks( $post->post_content );
    foreach ( $blocks as $block_index => $block ) {
        if ( ! empty( $block['blockName'] ) && $block['blockName'] === 'wp-3d-asset-editor/block' ) {
            $attrs = $block['attrs'] ?? [];
            $scene_id = intval( $attrs['sceneId'] ?? 0 );
            if ( $scene_id <= 0 ) continue;

            // generate a unique key per block instance
            $origin_key = $post_id . '-' . $block_index;

            // avoid creating more than one asset per block instance
            $existing = get_posts([
                'post_type' => '3d_asset',
                'meta_query' => [
                    [ 'key' => 'origin_post', 'value' => $origin_key, 'compare' => '=' ]
                ],
                'posts_per_page' => 1,
                'fields' => 'ids',
            ]);
            if ( ! empty( $existing ) ) continue;

            // get asset data from block attributes
            $assetUrl = sanitize_text_field( $attrs['assetUrl'] ?? '' );
            $posX = floatval( $attrs['posX'] ?? 0 );
            $posY = floatval( $attrs['posY'] ?? 0 );
            $posZ = floatval( $attrs['posZ'] ?? 0 );
            $rotX = floatval( $attrs['rotX'] ?? 0 );
            $rotY = floatval( $attrs['rotY'] ?? 0 );
            $rotZ = floatval( $attrs['rotZ'] ?? 0 );

            wp_insert_post([
                'post_type' => '3d_asset',
                'post_title' => sprintf('Asset from post #%d (block %d)', $post_id, $block_index),
                'post_status' => 'publish',
                'meta_input' => [
                    'assetUrl'    => $assetUrl,
                    'posX'        => $posX,
                    'posY'        => $posY,
                    'posZ'        => $posZ,
                    'rotX'        => $rotX,
                    'rotY'        => $rotY,
                    'rotZ'        => $rotZ,
                    'scene_id'    => $scene_id,
                    'origin_post' => $origin_key,
                ],
            ]);
        }
    }
}
add_action( 'save_post', 'wp3d_create_asset_from_post', 20, 3 );