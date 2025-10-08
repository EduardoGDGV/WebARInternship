<?php
/**
 * Plugin Name: WP 3D Asset Editor
 * Description: Manage 3D assets with positions and rotations via Gutenberg + Babylon.js.
 * Version: 1.4.0
 * Author: EduardoGDGV
 */

defined( 'ABSPATH' ) || exit;

/** Register block */
function wp3d_asset_editor_register_block() {
    register_block_type( __DIR__ . '/build' );
}
add_action( 'init', 'wp3d_asset_editor_register_block' );

/** Register CPT + Meta */
function wp3d_register_cpt_and_meta() {
    register_post_type('3d_asset', [
        'label'        => '3D Assets',
        'public'       => false,
        'show_ui'      => true,
        'show_in_rest' => true,
        'rest_base'    => '3d_asset',
        'supports'     => ['title', 'custom-fields'],
        'menu_icon'    => 'dashicons-cube',
    ]);

    // String field
    register_post_meta('3d_asset', 'assetUrl', [
        'type'              => 'string',
        'single'            => true,
        'show_in_rest'      => true,
        'sanitize_callback' => function( $value ) {
            return is_string( $value ) ? esc_url_raw( $value ) : '';
        },
        'auth_callback'     => function() {
            return current_user_can('edit_posts');
        },
    ]);

    // Number fields
    $number_fields = ['posX','posY','posZ','rotX','rotY','rotZ'];
    foreach ($number_fields as $field) {
        register_post_meta('3d_asset', $field, [
            'type'              => 'number',
            'single'            => true,
            'show_in_rest'      => true,
            'sanitize_callback' => function( $value ) {
                return is_numeric( $value ) ? floatval( $value ) : 0.0;
            },
            'auth_callback'     => function() {
                return current_user_can('edit_posts');
            },
        ]);
    }
}
add_action('init', 'wp3d_register_cpt_and_meta');

/** Add meta box for manual editing (optional, still useful in wp-admin UI) */
function wp3d_add_asset_meta_box() {
    add_meta_box(
        'wp3d_asset_meta',
        '3D Asset Settings',
        'wp3d_render_asset_meta_box',
        '3d_asset',
        'normal',
        'default'
    );
}
add_action('add_meta_boxes', 'wp3d_add_asset_meta_box');

function wp3d_render_asset_meta_box($post) {
    $fields = ['assetUrl','posX','posY','posZ','rotX','rotY','rotZ'];
    echo '<table class="widefat striped">';
    echo '<thead><tr><th>Field</th><th>Value</th></tr></thead><tbody>';
    foreach ($fields as $field) {
        $value = get_post_meta($post->ID, $field, true);
        echo '<tr>';
        echo '<td><strong>' . esc_html($field) . '</strong></td>';
        echo '<td>' . esc_html($value !== '' ? $value : '—') . '</td>';
        echo '</tr>';
    }
    echo '</tbody></table>';
    echo '<p><em>Values are set automatically when editing in the Gutenberg block. Editing here is disabled.</em></p>';
}

/** Allow .glb/.gltf uploads */
function wp3d_allow_3d_uploads($mime_types) {
    $mime_types['glb']  = 'model/gltf-binary';
    $mime_types['gltf'] = 'model/gltf+json';
    return $mime_types;
}
add_filter('upload_mimes', 'wp3d_allow_3d_uploads');

/** Allow admins unfiltered upload */
function wp3d_allow_unfiltered_uploads() {
    if ( current_user_can('administrator') ) {
        add_filter('user_has_cap', function($allcaps) {
            $allcaps['unfiltered_upload'] = true;
            return $allcaps;
        }, 0);
    }
}
add_action('init', 'wp3d_allow_unfiltered_uploads');
